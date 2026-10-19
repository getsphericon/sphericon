package oauthserver

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/apitoken"
	"github.com/getsphericon/sphericon/ent/oauthcode"
	"github.com/getsphericon/sphericon/ent/oauthrefreshtoken"
	"github.com/getsphericon/sphericon/internal/credentials"
)

const (
	grantAuthorizationCode = "authorization_code"
	grantRefreshToken      = "refresh_token"

	// accessTokenTTL is how long an issued access token authenticates (MCP: short-lived).
	accessTokenTTL = time.Hour
	// refreshTokenTTL is how long a refresh token can be exchanged; every rotation starts it anew.
	refreshTokenTTL = 30 * 24 * time.Hour
)

// errRefreshReuse means a refresh token was presented that is already used (or was claimed by a
// concurrent request): the grant is revoked.
var errRefreshReuse = errors.New("refresh token reuse")

// oauthError is a token endpoint failure rendered as an RFC 6749 section 5.2 response.
type oauthError struct {
	status      int
	kind        string
	description string
}

func (e *oauthError) Error() string { return e.kind + ": " + e.description }

func invalidGrant(description string) *oauthError {
	return &oauthError{http.StatusBadRequest, "invalid_grant", description}
}

// issued is a token response: the access token, its lifetime, and the refresh token that
// replaces it.
type issued struct {
	accessToken  string
	refreshToken string
	scopes       []string
}

// token serves the token endpoint: the authorization_code grant mints the connector's API token,
// the refresh_token grant rotates it.
func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "body must be form encoded")
		return
	}
	if res := r.PostForm.Get("resource"); res != "" && res != s.resource {
		writeOAuthError(w, http.StatusBadRequest, "invalid_target", "unknown resource")
		return
	}

	var (
		out *issued
		err error
	)
	switch r.PostForm.Get("grant_type") {
	case grantAuthorizationCode:
		out, err = s.exchangeCode(r.Context(), r.PostForm.Get("code"), r.PostForm.Get("client_id"),
			r.PostForm.Get("redirect_uri"), r.PostForm.Get("code_verifier"))
	case grantRefreshToken:
		out, err = s.refresh(r.Context(), r.PostForm.Get("refresh_token"), r.PostForm.Get("client_id"), r.PostForm.Get("scope"))
	default:
		err = &oauthError{http.StatusBadRequest, "unsupported_grant_type", "only authorization_code and refresh_token are supported"}
	}
	if oe, ok := errors.AsType[*oauthError](err); ok {
		writeOAuthError(w, oe.status, oe.kind, oe.description)
		return
	}
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  out.accessToken,
		"token_type":    "Bearer",
		"expires_in":    int(accessTokenTTL.Seconds()),
		"refresh_token": out.refreshToken,
		"scope":         strings.Join(out.scopes, " "),
	})
}

// exchangeCode trades an authorization code (with its PKCE verifier) for an API token that
// expires, plus the first refresh token of the grant.
func (s *Server) exchangeCode(ctx context.Context, code, clientID, redirectURI, verifier string) (*issued, error) {
	grant, err := s.ent.OAuthCode.Query().
		Where(oauthcode.CodeHash(hashSecret(code))).
		WithClient().
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, invalidGrant("unknown or expired code")
	}
	if err != nil {
		return nil, err
	}

	switch {
	case grant.Edges.Client.ClientID != clientID, grant.RedirectURI != redirectURI:
		return nil, invalidGrant("client or redirect_uri does not match the authorization request")
	case !validPKCE(verifier),
		subtle.ConstantTimeCompare([]byte(oauth2.S256ChallengeFromVerifier(verifier)), []byte(grant.CodeChallenge)) != 1:
		return nil, invalidGrant("code_verifier does not match")
	}

	prefix, err := credentials.GenerateTokenPrefix()
	if err != nil {
		return nil, err
	}
	secret, err := credentials.GenerateTokenSecret()
	if err != nil {
		return nil, err
	}
	hash, err := credentials.HashTokenSecret(secret)
	if err != nil {
		return nil, err
	}

	var refresh string
	err = s.inTx(ctx, func(tx *ent.Tx) error {
		// Single use: only the request that flips used_at wins.
		now := time.Now()
		claimed, err := tx.OAuthCode.Update().
			Where(oauthcode.ID(grant.ID), oauthcode.UsedAtIsNil(), oauthcode.ExpiresAtGT(now)).
			SetUsedAt(now).
			Save(ctx)
		if err != nil {
			return err
		}
		if claimed == 0 {
			return invalidGrant("unknown or expired code")
		}

		token, err := tx.ApiToken.Create().
			SetName(grant.Edges.Client.Name + " (MCP)").
			SetPrefix(prefix).
			SetSecretHash(hash).
			SetScopes(grant.Scopes).
			SetWorkspaceID(grant.WorkspaceID).
			SetExpiresAt(now.Add(accessTokenTTL)).
			Save(ctx)
		if err != nil {
			return err
		}
		refresh, err = newRefreshToken(ctx, tx, grant.ClientID, token.ID, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &issued{accessToken: credentials.TokenValue(prefix, secret), refreshToken: refresh, scopes: grant.Scopes}, nil
}

// refresh trades a refresh token for a new access token and the next refresh token. The grant's
// API token keeps its row (and prefix) in Settings → API keys; only its secret and expiry rotate.
// A refresh token is single use: presenting a used one revokes the grant.
func (s *Server) refresh(ctx context.Context, presented, clientID, scope string) (*issued, error) {
	row, err := s.ent.OAuthRefreshToken.Query().
		Where(oauthrefreshtoken.TokenHash(hashSecret(presented))).
		WithClient().
		WithAPIToken().
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, invalidGrant("unknown or expired refresh token")
	}
	if err != nil {
		return nil, err
	}
	token := row.Edges.APIToken

	switch {
	case row.Edges.Client.ClientID != clientID:
		return nil, invalidGrant("refresh token was issued to another client")
	case row.UsedAt != nil:
		return nil, s.revokeGrant(ctx, token.ID)
	case token.RevokedAt != nil, !row.ExpiresAt.After(time.Now()):
		return nil, invalidGrant("unknown or expired refresh token")
	}
	for _, want := range strings.Fields(scope) {
		if !slices.Contains(token.Scopes, want) {
			return nil, &oauthError{http.StatusBadRequest, "invalid_scope", "scope exceeds the original grant"}
		}
	}

	secret, err := credentials.GenerateTokenSecret()
	if err != nil {
		return nil, err
	}
	hash, err := credentials.HashTokenSecret(secret)
	if err != nil {
		return nil, err
	}

	var next string
	err = s.inTx(ctx, func(tx *ent.Tx) error {
		now := time.Now()
		claimed, err := tx.OAuthRefreshToken.Update().
			Where(oauthrefreshtoken.ID(row.ID), oauthrefreshtoken.UsedAtIsNil(), oauthrefreshtoken.ExpiresAtGT(now)).
			SetUsedAt(now).
			Save(ctx)
		if err != nil {
			return err
		}
		if claimed == 0 {
			return errRefreshReuse
		}
		rotated, err := tx.ApiToken.Update().
			Where(apitoken.ID(token.ID), apitoken.RevokedAtIsNil()).
			SetSecretHash(hash).
			SetExpiresAt(now.Add(accessTokenTTL)).
			Save(ctx)
		if err != nil {
			return err
		}
		if rotated == 0 {
			return invalidGrant("unknown or expired refresh token")
		}
		if _, err := tx.OAuthRefreshToken.Delete().
			Where(oauthrefreshtoken.APITokenID(token.ID), oauthrefreshtoken.ExpiresAtLT(now)).
			Exec(ctx); err != nil {
			return err
		}
		next, err = newRefreshToken(ctx, tx, row.ClientID, token.ID, now)
		return err
	})
	if errors.Is(err, errRefreshReuse) {
		return nil, s.revokeGrant(ctx, token.ID)
	}
	if err != nil {
		return nil, err
	}
	return &issued{accessToken: credentials.TokenValue(token.Prefix, secret), refreshToken: next, scopes: token.Scopes}, nil
}

// revokeGrant revokes the grant's API token (a rotated refresh token was replayed, so the
// connector's credentials may be stolen) and answers invalid_grant.
func (s *Server) revokeGrant(ctx context.Context, apiTokenID int64) error {
	_, err := s.ent.ApiToken.Update().
		Where(apitoken.ID(apiTokenID), apitoken.RevokedAtIsNil()).
		SetRevokedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return err
	}
	return invalidGrant("refresh token was already used; the grant is revoked")
}

// newRefreshToken stores a fresh refresh token for the grant and returns its plaintext.
func newRefreshToken(ctx context.Context, tx *ent.Tx, clientRowID, apiTokenID int64, now time.Time) (string, error) {
	plain := rand.Text()
	_, err := tx.OAuthRefreshToken.Create().
		SetTokenHash(hashSecret(plain)).
		SetClientID(clientRowID).
		SetAPITokenID(apiTokenID).
		SetExpiresAt(now.Add(refreshTokenTTL)).
		Save(ctx)
	return plain, err
}

// inTx runs fn in a transaction, committing when it returns nil.
func (s *Server) inTx(ctx context.Context, fn func(tx *ent.Tx) error) error {
	tx, err := s.ent.Tx(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}
