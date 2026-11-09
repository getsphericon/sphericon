// Package connectionissuance is the use case that issues a Connection (GLOSSARY, ADR 0016 phase
// 2, ADR 0061, ADR 0102) behind the OAuth token endpoint, the operation across modules of the OAuth
// server for MCP connectors: it claims an authorization code or a refresh token of
// internal/domain/identity/oauthclients and issues, rotates or revokes the Connection through
// internal/domain/identity/apitokens, an API token tied to the approving User's Membership, which
// /mcp and /api authenticate like a hand-made token and then bound by that User's current Role.
// The protocol's endpoints (discovery, registration, authorize and the token endpoint's request and
// answer) are the driver internal/api/oauth.
//
// A code or a refresh token is a secret that names its grant before any scope exists, so the use
// case holds the raw *ent.Client (one of the closed list of docs/architecture/workspace-scoping.md, Workspace
// scoping): it opens the exchange's transaction and scopes the Connection's writes to the
// Workspace the claimed grant names.
package connectionissuance

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	"github.com/samber/lo"
	"golang.org/x/oauth2"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/identity/oauthclients"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// AccessTokenTTL is how long an issued access token authenticates (MCP: short-lived).
const AccessTokenTTL = time.Hour

// errRefreshReuse means a refresh token was presented that is already used (or was claimed by a
// concurrent request): the grant is revoked.
var errRefreshReuse = errors.New("refresh token reuse")

// The refusals of an exchange (ADR 0104: the shared taxonomy), which the token endpoint answers
// in RFC 6749 section 5.2's words.
var (
	// ErrInvalidGrant is a code or a refresh token that names no grant this exchange may use:
	// unknown, expired, already used, or issued to another client or redirect URI (invalid_grant).
	ErrInvalidGrant = validation.UnknownReference("connectionissuance: invalid grant", i18n.ErrorsOauthGrantInvalid)
	// ErrScopeExceedsGrant is a refresh that asks for a scope the grant does not hold
	// (invalid_scope).
	ErrScopeExceedsGrant = validation.Refuse("connectionissuance: scope exceeds the original grant", i18n.ErrorsOauthScopeExceedsGrant)
)

var errUnknownRefreshToken = invalidGrant("unknown or expired refresh token")

// invalidGrant is ErrInvalidGrant with what was wrong with the grant.
func invalidGrant(description string) error { return ErrInvalidGrant.With(description) }

// Issued is what an exchange issues: the access token, the refresh token that replaces it and
// the scopes it carries.
type Issued struct {
	AccessToken  string
	RefreshToken string
	Scopes       []string
}

// UseCase issues, rotates and revokes the Connection of a grant.
type UseCase struct {
	ent     *ent.Client
	clients *oauthclients.Module
	now     func() time.Time
}

// New builds the use case over the ent client, which opens the transaction of an exchange, the
// OAuth clients module, which reads the codes and refresh tokens, and the Instance clock.
func New(client *ent.Client, clients *oauthclients.Module, now func() time.Time) *UseCase {
	return &UseCase{ent: client, clients: clients, now: now}
}

// ExchangeCodeInput is a token request with an authorization code: the code, the client it was
// issued to, the redirect URI of the authorization request and the PKCE verifier.
type ExchangeCodeInput struct {
	Code        string
	ClientID    string
	RedirectURI string
	Verifier    string
}

// RefreshInput is a token request with a refresh token: the token, the client it was issued to
// and the scope asked for (empty keeps the grant's).
type RefreshInput struct {
	RefreshToken string
	ClientID     string
	Scope        string
}

// ExchangeCode trades an authorization code (with its PKCE verifier) for a Connection, an API
// token of the approving User's Membership that expires, plus the first refresh token of the grant.
func (s *UseCase) ExchangeCode(ctx context.Context, in ExchangeCodeInput) (*Issued, error) {
	clientID, redirectURI, verifier := in.ClientID, in.RedirectURI, in.Verifier
	grant, err := s.clients.Code(ctx, in.Code)
	if ent.IsNotFound(err) {
		return nil, invalidGrant("unknown or expired code")
	}
	if err != nil {
		return nil, err
	}

	switch {
	case grant.Edges.Client.ClientID != clientID, grant.RedirectURI != redirectURI:
		return nil, invalidGrant("client or redirect_uri does not match the authorization request")
	case !oauthclients.ValidPKCE(verifier),
		subtle.ConstantTimeCompare([]byte(oauth2.S256ChallengeFromVerifier(verifier)), []byte(grant.CodeChallenge)) != 1:
		return nil, invalidGrant("code_verifier does not match")
	}

	var minted apitokens.Minted
	var refresh string
	err = s.inTx(ctx, func(tx *ent.Client) error {
		// Single use: only the request that flips used_at wins.
		now := s.now()
		claimed, err := oauthclients.ClaimCodeIn(ctx, tx, grant.ID, now)
		if err != nil {
			return err
		}
		if !claimed {
			return invalidGrant("unknown or expired code")
		}

		// The Connection of the Workspace the code names: the code is the secret that names it.
		minted, err = apitokens.IssueConnection(ctx, tx.Scoped(grant.WorkspaceID), apitokens.Grant{
			ClientName:    grant.Edges.Client.Name,
			Scopes:        grant.Scopes,
			MembershipID:  grant.MembershipID,
			OAuthClientID: grant.ClientID,
			ExpiresAt:     now.Add(AccessTokenTTL),
		})
		if err != nil {
			return err
		}
		refresh, err = oauthclients.NewRefreshTokenIn(ctx, tx, grant.ClientID, minted.Token.ID, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &Issued{AccessToken: minted.Value, RefreshToken: refresh, Scopes: grant.Scopes}, nil
}

// Refresh trades a refresh token for a new access token and the next refresh token. The
// Connection keeps its row (and prefix); only its secret and expiry rotate.
// A refresh token is single use: presenting a used one revokes the grant.
func (s *UseCase) Refresh(ctx context.Context, in RefreshInput) (*Issued, error) {
	clientID, scope := in.ClientID, in.Scope
	row, err := s.clients.RefreshToken(ctx, in.RefreshToken)
	if ent.IsNotFound(err) {
		return nil, errUnknownRefreshToken
	}
	if err != nil {
		return nil, err
	}
	token := row.Edges.APIToken

	switch {
	case row.Edges.Client.ClientID != clientID:
		return nil, invalidGrant("refresh token was issued to another client")
	case row.UsedAt != nil:
		return nil, s.revokeGrant(ctx, token)
	case token.RevokedAt != nil, !row.ExpiresAt.After(s.now()):
		return nil, errUnknownRefreshToken
	}
	// A scope parameter narrows the grant for good (RFC 6749 section 6); it never widens it.
	scopes := token.Scopes
	if want := lo.Uniq(strings.Fields(scope)); len(want) > 0 {
		if !lo.Every(token.Scopes, want) {
			return nil, ErrScopeExceedsGrant
		}
		scopes = want
	}

	var access, next string
	err = s.inTx(ctx, func(tx *ent.Client) error {
		now := s.now()
		claimed, err := oauthclients.ClaimRefreshTokenIn(ctx, tx, row.ID, now)
		if err != nil {
			return err
		}
		if !claimed {
			return errRefreshReuse
		}
		// The Connection's Workspace: the refresh token is the secret that names its grant.
		value, rotated, err := apitokens.RotateConnection(ctx, tx.Scoped(token.WorkspaceID), token, scopes, now.Add(AccessTokenTTL))
		if err != nil {
			return err
		}
		if !rotated {
			return errUnknownRefreshToken
		}
		access = value
		if err := oauthclients.PruneRefreshTokensIn(ctx, tx, token.ID, now); err != nil {
			return err
		}
		next, err = oauthclients.NewRefreshTokenIn(ctx, tx, row.ClientID, token.ID, now)
		return err
	})
	if errors.Is(err, errRefreshReuse) {
		return nil, s.revokeGrant(ctx, token)
	}
	if err != nil {
		return nil, err
	}
	return &Issued{AccessToken: access, RefreshToken: next, Scopes: scopes}, nil
}

// revokeGrant revokes the grant's API token (a rotated refresh token was replayed, so the
// connector's credentials may be stolen) and answers invalid_grant.
func (s *UseCase) revokeGrant(ctx context.Context, token *ent.ApiToken) error {
	if err := apitokens.RevokeGrant(ctx, s.ent.Scoped(token.WorkspaceID), token.ID, s.now()); err != nil {
		return err
	}
	return invalidGrant("refresh token was already used; the grant is revoked")
}

// inTx runs fn in a transaction, committing when it returns nil.
func (s *UseCase) inTx(ctx context.Context, fn func(tx *ent.Client) error) error {
	tx, err := s.ent.Tx(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx.Client()); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}
