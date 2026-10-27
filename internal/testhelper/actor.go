package testhelper

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/user"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	apiauth "github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/fixtures"
	gptoken "github.com/go-pkgz/auth/v2/token"
	"github.com/golang-jwt/jwt/v5"
	ht "github.com/ogen-go/ogen/http"
	"github.com/stretchr/testify/require"
)

// An actor is a ready typed client for one API surface acting as one identity.
// JWT signing, token generation and hashing, security sources and the in-memory
// transport are internal to this file; tests only pick the identity.

const (
	siteBase     = "http://local/site"
	externalBase = "http://local/api"
	collectBase  = "http://local/collect"
)

// SiteActor returns a /site client authenticated as the fixture user with the
// given login email (use the fixtures.*Email constants). Each request carries a
// session token minted for it (see SiteToken); hold a SiteToken for a session
// that must outlive an epoch bump.
func (env *TestEnv) SiteActor(t *testing.T, email string) *siteapi.Client {
	t.Helper()
	c, err := siteapi.NewClient(siteBase, sessionSource{env, email}, siteapi.WithClient(env.Transport(nil)))
	require.NoError(t, err)
	return c
}

// SiteWithToken returns a /site client carrying the given raw session token (for
// tests of tokens in states a login never issues, minted with SiteToken).
func (env *TestEnv) SiteWithToken(t *testing.T, token string) *siteapi.Client {
	t.Helper()
	return env.siteClient(t, token)
}

// SiteActorVia is SiteActor with the in-memory transport wrapped by wrap, for
// tests that observe the wire (e.g. raw response bodies).
func (env *TestEnv) SiteActorVia(t *testing.T, email string, wrap func(inner ht.Client) ht.Client) *siteapi.Client {
	t.Helper()
	c, err := siteapi.NewClient(siteBase, sessionSource{env, email}, siteapi.WithClient(wrap(env.Transport(nil))))
	require.NoError(t, err)
	return c
}

// SiteAnonymous returns a /site client carrying no credential.
func (env *TestEnv) SiteAnonymous(t *testing.T) *siteapi.Client {
	t.Helper()
	return env.siteClient(t, "")
}

// ExternalAnchor returns an /api client bearing the fixture anchor token
// (scopes contacts, tokens and events, read and write).
func (env *TestEnv) ExternalAnchor(t *testing.T) *externalapi.Client {
	t.Helper()
	return env.ExternalWithToken(t, credentials.TokenValue(fixtures.AnchorTokenPrefix, fixtures.AnchorTokenSecret))
}

// ExternalScoped returns an /api client bearing a fresh Acme token that has
// exactly the given scopes, created through the production token code.
func (env *TestEnv) ExternalScoped(t *testing.T, scopes ...string) *externalapi.Client {
	t.Helper()
	return env.ExternalWithToken(t, env.ScopedBearer(t, scopes...))
}

// ScopedBearer returns the raw bearer value of a fresh Acme token that has
// exactly the given scopes, for tests that must speak raw HTTP or MCP.
func (env *TestEnv) ScopedBearer(t *testing.T, scopes ...string) string {
	t.Helper()
	return env.ScopedBearerFor(t, fixtures.AcmeID, scopes...)
}

// ScopedBearerFor is ScopedBearer for another fixture Workspace (tenant-isolation
// tests, e.g. fixtures.GlobexID).
func (env *TestEnv) ScopedBearerFor(t *testing.T, workspaceID int64, scopes ...string) string {
	t.Helper()
	prefix, err := credentials.GenerateTokenPrefix()
	require.NoError(t, err)
	secret, err := credentials.GenerateTokenSecret()
	require.NoError(t, err)
	_, err = env.DB.ApiToken.Create().
		SetName("actor-token").SetPrefix(prefix).SetSecretHash(credentials.HashSecret(secret)).SetScopes(scopes).
		SetWorkspaceID(workspaceID).
		Save(t.Context())
	require.NoError(t, err)
	return credentials.TokenValue(prefix, secret)
}

// ExternalAnonymous returns an /api client carrying no credential.
func (env *TestEnv) ExternalAnonymous(t *testing.T) *externalapi.Client {
	t.Helper()
	return env.ExternalWithToken(t, "")
}

// ExternalWithToken returns an /api client bearing the given raw token value
// (for wrong-credential tests).
func (env *TestEnv) ExternalWithToken(t *testing.T, bearer string) *externalapi.Client {
	t.Helper()
	c, err := externalapi.NewClient(externalBase, bearerSource{bearer}, externalapi.WithClient(env.Transport(nil)))
	require.NoError(t, err)
	return c
}

// ExternalVia is ExternalWithToken with the in-memory transport wrapped by wrap, for
// tests that change the request on the wire (e.g. RewriteURL).
func (env *TestEnv) ExternalVia(t *testing.T, bearer string, wrap func(inner ht.Client) ht.Client) *externalapi.Client {
	t.Helper()
	c, err := externalapi.NewClient(externalBase, bearerSource{bearer}, externalapi.WithClient(wrap(env.Transport(nil))))
	require.NoError(t, err)
	return c
}

// RewriteURL wraps a transport so every occurrence of from in a request's URL (path and
// query) becomes to. A typed client cannot spell a malformed id, so a test sends a valid
// sentinel id and swaps it on the wire.
func RewriteURL(from, to string) func(inner ht.Client) ht.Client {
	return func(inner ht.Client) ht.Client {
		return rewriteClient{inner: inner, from: from, to: to}
	}
}

type rewriteClient struct {
	inner    ht.Client
	from, to string
}

func (c rewriteClient) Do(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.URL.Path = strings.ReplaceAll(r.URL.Path, c.from, c.to)
	r.URL.RawPath = ""
	r.URL.RawQuery = strings.ReplaceAll(r.URL.RawQuery, c.from, c.to)
	return c.inner.Do(r)
}

// CollectAcme returns a /collect client carrying the Acme collect key.
func (env *TestEnv) CollectAcme(t *testing.T) *collectapi.Client {
	t.Helper()
	return env.CollectWithKey(t, fixtures.AcmeCollectKey)
}

// CollectAnonymous returns a /collect client carrying no key.
func (env *TestEnv) CollectAnonymous(t *testing.T) *collectapi.Client {
	t.Helper()
	return env.CollectWithKey(t, "")
}

// CollectWithKey returns a /collect client carrying the given raw key (for
// wrong-credential tests).
func (env *TestEnv) CollectWithKey(t *testing.T, key string) *collectapi.Client {
	t.Helper()
	c, err := collectapi.NewClient(collectBase, collectKeySource{key}, collectapi.WithClient(env.Transport(nil)))
	require.NoError(t, err)
	return c
}

func (env *TestEnv) siteClient(t *testing.T, jwtValue string) *siteapi.Client {
	t.Helper()
	c, err := siteapi.NewClient(siteBase, cookieSource{jwtValue}, siteapi.WithClient(env.Transport(nil)))
	require.NoError(t, err)
	return c
}

// SiteToken mints the session token a login issues for the fixture user with the
// given email: the go-pkgz claims, stamped by the production apiauth.StampUser
// (User id and current session epoch), signed with the test config's secret and
// valid for SPHERICON_SESSION_TTL from the env's clock. edit, when not nil, changes the
// claims before signing, for tests of tokens a login never issues (expired,
// without an epoch). The token is fixed at minting: a later epoch bump ends it.
func (env *TestEnv) SiteToken(t *testing.T, email string, edit func(*gptoken.Claims)) string {
	t.Helper()
	tk, err := env.mintSiteToken(t.Context(), email, edit)
	require.NoError(t, err)
	return tk
}

func (env *TestEnv) mintSiteToken(ctx context.Context, email string, edit func(*gptoken.Claims)) (string, error) {
	svc := gptoken.NewService(gptoken.Opts{
		SecretReader: gptoken.SecretFunc(func(string) (string, error) { return env.jwtSecret, nil }),
		Issuer:       "sphericon",
		DisableXSRF:  true,
	})
	claims := gptoken.Claims{
		Issuer:    "sphericon",
		Audience:  jwt.ClaimStrings{"sphericon"},
		ExpiresAt: jwt.NewNumericDate(env.now().Add(env.sessionTTL)),
		User:      &gptoken.User{Name: email, ID: "test"},
	}
	// An unknown email leaves the claims unstamped: the site rejects the token.
	if u, err := env.DB.User.Query().Where(user.Email(email)).Only(ctx); err == nil {
		apiauth.StampUser(&claims, u)
		claims.User.SetStrAttr(apiauth.ClaimMethod, string(apiauth.MethodPassword))
	} else if !ent.IsNotFound(err) {
		return "", err
	}
	if edit != nil {
		edit(&claims)
	}
	return svc.Token(claims)
}

// sessionSource signs an actor in afresh on every request, under the request's
// context, so the actor always holds a current session.
type sessionSource struct {
	env   *TestEnv
	email string
}

func (s sessionSource) ApiKeyAuth(ctx context.Context, _ siteapi.OperationName) (siteapi.ApiKeyAuth, error) {
	tk, err := s.env.mintSiteToken(ctx, s.email, nil)
	return siteapi.ApiKeyAuth{APIKey: tk}, err
}

type cookieSource struct{ token string }

func (s cookieSource) ApiKeyAuth(context.Context, siteapi.OperationName) (siteapi.ApiKeyAuth, error) {
	return siteapi.ApiKeyAuth{APIKey: s.token}, nil
}

type bearerSource struct{ token string }

func (s bearerSource) BearerAuth(context.Context, externalapi.OperationName) (externalapi.BearerAuth, error) {
	return externalapi.BearerAuth{Token: s.token}, nil
}

type collectKeySource struct{ key string }

func (s collectKeySource) ApiKeyAuth(context.Context, collectapi.OperationName) (collectapi.ApiKeyAuth, error) {
	return collectapi.ApiKeyAuth{APIKey: s.key}, nil
}
