package apitokens

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/apitoken"
	"github.com/getsphericon/sphericon/ent/oauthrefreshtoken"
	"github.com/getsphericon/sphericon/internal/domain/identity/oauthclients"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

const (
	// AccessTokenTTL is how long an issued access token authenticates (MCP: short-lived).
	AccessTokenTTL = time.Hour
	// refreshTokenTTL is how long a refresh token can be exchanged; every rotation starts it anew.
	refreshTokenTTL = 30 * 24 * time.Hour
)

// ErrScopeExceedsGrant is a refresh that asks for a scope the grant does not hold (RFC 6749
// section 5.2, invalid_scope). Every other refusal of an exchange is oauthclients.ErrInvalidGrant.
var ErrScopeExceedsGrant = validation.Refuse("apitokens: scope exceeds the original grant", i18n.ErrorsOauthScopeExceedsGrant)

var (
	errUnknownRefreshToken = oauthclients.InvalidGrant("unknown or expired refresh token")
	// errRefreshReuse means a refresh token was presented that is already used (or was claimed by
	// a concurrent request): the grant is revoked.
	errRefreshReuse = errors.New("refresh token reuse")
)

// Module issues the tokens whose request names no Workspace scope yet: it exchanges an
// authorization code or a refresh token for a Connection (ADR 0016, ADR 0061), the token
// endpoint's operations, and mints the first token of an Instance by its bootstrap token
// (Bootstrap). A code or a refresh token is a secret that names its grant before any Workspace
// scope exists, so the module holds the raw *ent.Client (one of the closed list of
// docs/architecture/workspace-scoping.md, Workspace scoping): it opens the exchange's transaction and scopes the
// Connection's writes to the Workspace the claimed grant names.
type Module struct {
	ent        *ent.Client
	workspaces *workspaces.Module
	// bootstrapToken is the Instance's bootstrap token (SPHERICON_BOOTSTRAP_TOKEN), "" for none.
	bootstrapToken string
	now            func() time.Time
}

// New builds the module over the ent client, which opens the transaction of an exchange, the
// Workspaces the bootstrap mints into, the Instance's bootstrap token ("" when it has none) and the
// Instance clock.
func New(client *ent.Client, ws *workspaces.Module, bootstrapToken string, now func() time.Time) *Module {
	return &Module{ent: client, workspaces: ws, bootstrapToken: bootstrapToken, now: now}
}

// Issued is what an exchange issues: the access token, the refresh token that replaces it and
// the scopes it carries.
type Issued struct {
	AccessToken  string
	RefreshToken string
	Scopes       []string
}

// RefreshInput is a token request with a refresh token: the token, the client it was issued to
// and the scope asked for (empty keeps the grant's).
type RefreshInput struct {
	RefreshToken string
	ClientID     string
	Scope        string
}

// ExchangeCode trades an authorization code (with its PKCE verifier) for a Connection, an API
// token of the approving User's Membership that expires, plus the first refresh token of the
// grant. The code is claimed through oauthclients.ClaimCodeIn, which checks the request first.
func (m *Module) ExchangeCode(ctx context.Context, in oauthclients.CodeClaim) (*Issued, error) {
	var out *Issued
	err := m.inTx(ctx, func(tx *ent.Client) error {
		now := m.now()
		grant, err := oauthclients.ClaimCodeIn(ctx, tx, in, now)
		if err != nil {
			return err
		}
		// The Connection of the Workspace the code names: the code is the secret that names it.
		minted, err := issueConnection(ctx, tx.Scoped(grant.WorkspaceID), grant, now.Add(AccessTokenTTL))
		if err != nil {
			return err
		}
		refresh, err := newRefreshToken(ctx, tx, grant.ClientID, minted.Token.ID, now)
		if err != nil {
			return err
		}
		out = &Issued{AccessToken: minted.Value, RefreshToken: refresh, Scopes: grant.Scopes}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Refresh trades a refresh token for a new access token and the next refresh token. The
// Connection keeps its row (and prefix); only its secret and expiry rotate.
// A refresh token is single use: presenting a used one revokes the grant.
func (m *Module) Refresh(ctx context.Context, in RefreshInput) (*Issued, error) {
	row, err := m.ent.OAuthRefreshToken.Query().
		Where(oauthrefreshtoken.TokenHash(credentials.HashSecret(in.RefreshToken))).
		WithClient().
		WithAPIToken().
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, errUnknownRefreshToken
	}
	if err != nil {
		return nil, err
	}
	token := row.Edges.APIToken

	switch {
	case row.Edges.Client.ClientID != in.ClientID:
		return nil, oauthclients.InvalidGrant("refresh token was issued to another client")
	case row.UsedAt != nil:
		return nil, m.revokeGrant(ctx, token)
	case token.RevokedAt != nil, !row.ExpiresAt.After(m.now()):
		return nil, errUnknownRefreshToken
	}
	// A scope parameter narrows the grant for good (RFC 6749 section 6); it never widens it.
	scopes := token.Scopes
	if want := lo.Uniq(strings.Fields(in.Scope)); len(want) > 0 {
		if !lo.Every(token.Scopes, want) {
			return nil, ErrScopeExceedsGrant
		}
		scopes = want
	}

	var access, next string
	err = m.inTx(ctx, func(tx *ent.Client) error {
		now := m.now()
		// Single use: only the request that flips used_at wins.
		err := lifecycle.Latch(ctx, func(ctx context.Context) (bool, error) {
			claimed, err := tx.OAuthRefreshToken.Update().
				Where(oauthrefreshtoken.ID(row.ID), oauthrefreshtoken.UsedAtIsNil(), oauthrefreshtoken.ExpiresAtGT(now)).
				SetUsedAt(now).
				Save(ctx)
			return claimed > 0, err
		})
		if errors.Is(err, lifecycle.ErrInvalidTransition) {
			return errRefreshReuse
		}
		if err != nil {
			return err
		}
		// The Connection's Workspace: the refresh token is the secret that names its grant.
		value, rotated, err := rotateConnection(ctx, tx.Scoped(token.WorkspaceID), token, scopes, now.Add(AccessTokenTTL))
		if err != nil {
			return err
		}
		if !rotated {
			return errUnknownRefreshToken
		}
		access = value
		// The grant's refresh tokens that expired go as it rotates.
		if _, err := tx.OAuthRefreshToken.Delete().
			Where(oauthrefreshtoken.APITokenID(token.ID), oauthrefreshtoken.ExpiresAtLT(now)).
			Exec(ctx); err != nil {
			return err
		}
		next, err = newRefreshToken(ctx, tx, row.ClientID, token.ID, now)
		return err
	})
	if errors.Is(err, errRefreshReuse) {
		return nil, m.revokeGrant(ctx, token)
	}
	if err != nil {
		return nil, err
	}
	return &Issued{AccessToken: access, RefreshToken: next, Scopes: scopes}, nil
}

// issueConnection stores the Connection of the claimed code grant in the Workspace s, expiring at
// expiresAt, and returns it with its secret: the API token of the approving User's Membership, so
// it ends with it, named after the client and naming it, which tells it apart from a Personal
// token. The scopes are the consent's, which oauthclients checked against the User's Role.
func issueConnection(ctx context.Context, s *ent.Scoped, grant *ent.OAuthCode, expiresAt time.Time) (Minted, error) {
	prefix := credentials.GenerateTokenPrefix()
	secret := credentials.GenerateTokenSecret()
	token, err := s.ApiToken().Create().
		SetName(grant.Edges.Client.Name).
		SetPrefix(prefix).
		SetSecretHash(credentials.HashSecret(secret)).
		SetScopes(grant.Scopes).
		SetMembershipID(grant.MembershipID).
		SetOauthClientID(grant.ClientID).
		SetExpiresAt(expiresAt).
		Save(ctx)
	if err != nil {
		return Minted{}, err
	}
	return Minted{Token: token, Value: credentials.TokenValue(prefix, secret)}, nil
}

// rotateConnection gives the active Connection token of the Workspace s a fresh secret, scopes
// and expiry, keeping its row and prefix, and returns its new value; false when it was revoked
// meanwhile.
func rotateConnection(ctx context.Context, s *ent.Scoped, token *ent.ApiToken, scopes []string, expiresAt time.Time) (value string, rotated bool, err error) {
	secret := credentials.GenerateTokenSecret()
	n, err := s.ApiToken().Update().
		Where(apitoken.ID(token.ID), apitoken.RevokedAtIsNil()).
		SetSecretHash(credentials.HashSecret(secret)).
		SetScopes(scopes).
		SetExpiresAt(expiresAt).
		Save(ctx)
	if err != nil || n == 0 {
		return "", false, err
	}
	return credentials.TokenValue(token.Prefix, secret), true, nil
}

// newRefreshToken stores a fresh refresh token of the client clientRowID for the grant of the
// Connection apiTokenID on the transaction tx, valid from now, and returns its plaintext.
func newRefreshToken(ctx context.Context, tx *ent.Client, clientRowID, apiTokenID int64, now time.Time) (string, error) {
	plain := rand.Text()
	_, err := tx.OAuthRefreshToken.Create().
		SetTokenHash(credentials.HashSecret(plain)).
		SetClientID(clientRowID).
		SetAPITokenID(apiTokenID).
		SetExpiresAt(now.Add(refreshTokenTTL)).
		Save(ctx)
	return plain, err
}

// revokeGrant revokes the grant's Connection unless it is revoked already (a rotated refresh token
// was replayed, so the connector's credentials may be stolen, ADR 0016) and answers invalid_grant.
func (m *Module) revokeGrant(ctx context.Context, token *ent.ApiToken) error {
	err := lifecycle.Latch(ctx, func(ctx context.Context) (bool, error) {
		n, err := m.ent.Scoped(token.WorkspaceID).ApiToken().Update().
			Where(apitoken.ID(token.ID), apitoken.RevokedAtIsNil()).
			SetRevokedAt(m.now()).
			Save(ctx)
		return n > 0, err
	})
	// A grant revoked meanwhile is what this wanted.
	if err != nil && !errors.Is(err, lifecycle.ErrInvalidTransition) {
		return err
	}
	return oauthclients.InvalidGrant("refresh token was already used; the grant is revoked")
}

// inTx runs fn in a transaction, committing when it returns nil.
func (m *Module) inTx(ctx context.Context, fn func(tx *ent.Client) error) error {
	tx, err := m.ent.Tx(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx.Client()); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}
