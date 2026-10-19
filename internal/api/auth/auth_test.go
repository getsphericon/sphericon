package auth_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	gptoken "github.com/go-pkgz/auth/v2/token"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/apitoken"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// closedClient is an ent client whose database is gone: every query fails.
func closedClient(t *testing.T) *ent.Client {
	t.Helper()
	sqlDB, err := sql.Open("pgx", "postgres://closed.invalid/none")
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	return db.NewEntClient(sqlDB)
}

func TestHasScopeAndWorkspaceIDOfTheTokenContext(t *testing.T) {
	a := &auth.TokenAuth{TokenID: 7, WorkspaceID: 3, Scopes: []string{"contacts:read", "events:write"}}

	assert.True(t, auth.HasScope(a, "contacts:read"))
	assert.True(t, auth.HasScope(a, "events:write"))
	assert.False(t, auth.HasScope(a, "contacts:write"), "a read scope does not imply write")
	assert.False(t, auth.HasScope(a, ""))
	assert.False(t, auth.HasScope(nil, "contacts:read"), "no token has no scopes")

	assert.EqualValues(t, 3, a.WorkspaceID)
	assert.Nil(t, auth.TokenScoped(context.Background()), "unauthenticated has no scope, never a real tenant")
}

func TestAuthContextsRoundTrip(t *testing.T) {
	ctx := context.Background()
	assert.Nil(t, auth.GetTokenAuth(ctx))
	assert.Nil(t, auth.GetCollectAuth(ctx))
	assert.Nil(t, auth.GetSiteAuth(ctx))
	assert.Zero(t, collectWorkspaceID(ctx))

	token := &auth.TokenAuth{TokenID: 1, WorkspaceID: 2}
	assert.Same(t, token, auth.GetTokenAuth(auth.WithTokenAuth(ctx, token)))

	collect := &auth.CollectAuth{WorkspaceID: 5}
	withCollect := auth.WithCollectAuth(ctx, collect)
	assert.Same(t, collect, auth.GetCollectAuth(withCollect))
	assert.EqualValues(t, 5, collectWorkspaceID(withCollect))

	site := &auth.SiteAuth{UserID: 9, Email: "u@example.com"}
	assert.Same(t, site, auth.GetSiteAuth(auth.WithSiteAuth(ctx, site)))

	// The three identities live under distinct keys: one never reads as another.
	assert.Nil(t, auth.GetTokenAuth(withCollect))
	assert.Nil(t, auth.GetSiteAuth(withCollect))
}

func TestBearerAuthResolvesAValidTokenToItsWorkspaceAndScopes(t *testing.T) {
	env := testhelper.Setup(t)
	h := auth.NewExternalSecurityHandler(env.DB, env.Bus)

	ctx, err := h.HandleBearerAuth(context.Background(), "", externalapi.BearerAuth{
		Token: credentials.TokenValue(fixtures.AnchorTokenPrefix, fixtures.AnchorTokenSecret),
	})
	require.NoError(t, err)
	got := auth.GetTokenAuth(ctx)
	require.NotNil(t, got)
	assert.EqualValues(t, fixtures.AnchorTokenID, got.TokenID)
	assert.EqualValues(t, fixtures.AcmeID, got.WorkspaceID)
	assert.Equal(t, fixtures.AnchorTokenName, got.Name)
	assert.Contains(t, got.Scopes, "contacts:read")

	scoped := env.ScopedBearerFor(t, fixtures.GlobexID, "events:read")
	ctx, err = h.HandleBearerAuth(context.Background(), "", externalapi.BearerAuth{Token: scoped})
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.GlobexID, auth.GetTokenAuth(ctx).WorkspaceID, "the token's own workspace, whichever it is")
	assert.Equal(t, []string{"events:read"}, auth.GetTokenAuth(ctx).Scopes)
}

func TestBearerAuthRejectsEveryUnusableToken(t *testing.T) {
	env := testhelper.Setup(t)
	h := auth.NewExternalSecurityHandler(env.DB, env.Bus)
	ctx := context.Background()
	valid := credentials.TokenValue(fixtures.AnchorTokenPrefix, fixtures.AnchorTokenSecret)

	revoked := env.ScopedBearer(t, "contacts:read")
	parsed := credentials.ParseToken(revoked)
	require.NotNil(t, parsed)
	n, err := env.DB.ApiToken.Update().Where(apitoken.Prefix(parsed.Prefix)).SetRevokedAt(time.Now()).Save(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	expired := env.ScopedBearer(t, "contacts:read")
	parsed = credentials.ParseToken(expired)
	require.NotNil(t, parsed)
	n, err = env.DB.ApiToken.Update().Where(apitoken.Prefix(parsed.Prefix)).SetExpiresAt(time.Now().Add(-time.Minute)).Save(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	for name, token := range map[string]string{
		"empty":            "",
		"malformed":        "not-a-token",
		"unknown prefix":   credentials.TokenValue("nosuchprefix", "whatever"),
		"wrong secret":     credentials.TokenValue(fixtures.AnchorTokenPrefix, "not-the-secret"),
		"revoked":          revoked,
		"expired":          expired,
		"valid prefix cut": valid[:len(valid)-len(fixtures.AnchorTokenSecret)-1],
	} {
		got, err := h.HandleBearerAuth(ctx, "", externalapi.BearerAuth{Token: token})
		require.ErrorIs(t, err, auth.ErrUnauthorized, name)
		assert.Nil(t, auth.GetTokenAuth(got), name)
	}
}

func TestBearerAuthStorageFailureIsNotAnUnauthorizedAnswer(t *testing.T) {
	h := auth.NewExternalSecurityHandler(closedClient(t), nil)
	_, err := h.HandleBearerAuth(context.Background(), "", externalapi.BearerAuth{
		Token: credentials.TokenValue(fixtures.AnchorTokenPrefix, fixtures.AnchorTokenSecret),
	})
	require.Error(t, err)
	assert.NotErrorIs(t, err, auth.ErrUnauthorized, "a database outage is a 500, not a 401 for a valid token")
}

func TestCollectKeyResolvesTheOwningWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	h := auth.NewCollectSecurityHandler(env.DB)

	for key, ws := range map[string]int64{fixtures.AcmeCollectKey: fixtures.AcmeID, fixtures.GlobexCollectKey: fixtures.GlobexID} {
		ctx, err := h.HandleApiKeyAuth(context.Background(), "", collectapi.ApiKeyAuth{APIKey: key})
		require.NoError(t, err)
		assert.EqualValues(t, ws, collectWorkspaceID(ctx))
	}

	for name, key := range map[string]string{"empty": "", "unknown": "omck_nope", "ingest key is not a collect key": fixtures.IntegrationAcmeGenericAIngestKey} {
		ctx, err := h.HandleApiKeyAuth(context.Background(), "", collectapi.ApiKeyAuth{APIKey: key})
		require.ErrorIs(t, err, auth.ErrUnauthorized, name)
		assert.Zero(t, collectWorkspaceID(ctx), name)
	}

	_, err := auth.NewCollectSecurityHandler(closedClient(t)).HandleApiKeyAuth(context.Background(), "", collectapi.ApiKeyAuth{APIKey: fixtures.AcmeCollectKey})
	require.Error(t, err)
	assert.NotErrorIs(t, err, auth.ErrUnauthorized)
}

func TestSiteAuthResolvesTheDashboardUserByTheIDInTheToken(t *testing.T) {
	env := testhelper.Setup(t)
	cfg, err := config.Load("test")
	require.NoError(t, err)
	h := auth.NewSiteSecurityHandler(cfg.JWTSecret, env.DB, nil)

	ctx, err := h.HandleApiKeyAuth(context.Background(), "", siteapi.ApiKeyAuth{
		APIKey: env.SiteToken(t, fixtures.OwnerJohnEmail, nil),
	})
	require.NoError(t, err)
	got := auth.GetSiteAuth(ctx)
	require.NotNil(t, got)
	assert.EqualValues(t, fixtures.OwnerJohnID, got.UserID)
	assert.Equal(t, fixtures.OwnerJohnEmail, got.Email)

	// The id decides, not the login name the token also carries.
	ctx, err = h.HandleApiKeyAuth(context.Background(), "", siteapi.ApiKeyAuth{
		APIKey: env.SiteToken(t, fixtures.OwnerJaneEmail, func(c *gptoken.Claims) { c.User.Name = fixtures.OwnerJohnEmail }),
	})
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.OwnerJaneID, auth.GetSiteAuth(ctx).UserID)
}

func TestSiteAuthRejectsEveryUnusableCredential(t *testing.T) {
	env := testhelper.Setup(t)
	cfg, err := config.Load("test")
	require.NoError(t, err)
	h := auth.NewSiteSecurityHandler(cfg.JWTSecret, env.DB, nil)
	other := auth.NewSiteSecurityHandler("a-different-secret", env.DB, nil)
	good := env.SiteToken(t, fixtures.OwnerJohnEmail, nil)
	edited := func(edit func(*gptoken.Claims)) string { return env.SiteToken(t, fixtures.OwnerJohnEmail, edit) }

	for name, tc := range map[string]struct {
		handler *auth.SiteSecurityHandler
		token   string
	}{
		"empty":            {h, ""},
		"garbage":          {h, "not.a.jwt"},
		"signed elsewhere": {other, good},
		"no user claim":    {h, edited(func(c *gptoken.Claims) { c.User = nil })},
		"unknown login":    {h, env.SiteToken(t, "ghost@nowhere.test", nil)},
		"unknown user id":  {h, edited(func(c *gptoken.Claims) { c.User.SetStrAttr(auth.ClaimUserID, "999999999") })},
		"no user id":       {h, edited(func(c *gptoken.Claims) { delete(c.User.Attributes, auth.ClaimUserID) })},
		"no epoch":         {h, edited(func(c *gptoken.Claims) { delete(c.User.Attributes, auth.ClaimEpoch) })},
		"other epoch":      {h, edited(func(c *gptoken.Claims) { c.User.SetStrAttr(auth.ClaimEpoch, "7") })},
		"expired":          {h, edited(func(c *gptoken.Claims) { c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Second)) })},
		"no expiry":        {h, edited(func(c *gptoken.Claims) { c.ExpiresAt = nil })},
	} {
		ctx, err := tc.handler.HandleApiKeyAuth(context.Background(), "", siteapi.ApiKeyAuth{APIKey: tc.token})
		require.ErrorIs(t, err, auth.ErrUnauthorized, name)
		assert.Nil(t, auth.GetSiteAuth(ctx), name)
	}

	_, err = auth.NewSiteSecurityHandler(cfg.JWTSecret, closedClient(t), nil).HandleApiKeyAuth(context.Background(), "", siteapi.ApiKeyAuth{APIKey: good})
	require.Error(t, err)
	assert.NotErrorIs(t, err, auth.ErrUnauthorized)
}

// collectWorkspaceID is the resolved collect key's Workspace id, 0 when unauthenticated.
func collectWorkspaceID(ctx context.Context) int64 {
	if a := auth.GetCollectAuth(ctx); a != nil {
		return a.WorkspaceID
	}
	return 0
}

// A rotation: sessions signed with the old secret keep working while it is the secondary key, and
// stop once it is removed.
func TestSiteAuthAcceptsASessionSignedWithTheSecondarySecretOnlyWhileItIsConfigured(t *testing.T) {
	env := testhelper.Setup(t)
	cfg, err := config.Load("test")
	require.NoError(t, err)
	old := env.SiteToken(t, fixtures.OwnerJohnEmail, nil)
	const rotated = "the-new-secret-the-new-secret-0123"

	during := auth.NewSiteSecurityHandler(rotated, env.DB, nil, cfg.JWTSecret)
	_, err = during.Verify(context.Background(), old)
	require.NoError(t, err)

	after := auth.NewSiteSecurityHandler(rotated, env.DB, nil)
	_, err = after.Verify(context.Background(), old)
	require.ErrorIs(t, err, auth.ErrUnauthorized)

	// New sessions are signed with the primary secret and verify without the secondary one.
	cookie, err := auth.NewSessions(rotated, time.Hour, false, nil, cfg.JWTSecret).Issue(&ent.User{ID: fixtures.OwnerJohnID})
	require.NoError(t, err)
	_, err = after.Verify(context.Background(), cookie.Value)
	require.NoError(t, err)
}

func TestSessionsHolderAcceptsTheSecondarySecret(t *testing.T) {
	user := &ent.User{ID: 7, SessionEpoch: 3}
	cookie, err := auth.NewSessions("old-secret-old-secret-old-secret-0", time.Hour, false, nil).Issue(user)
	require.NoError(t, err)

	id, epoch, ok := auth.NewSessions("new-secret-new-secret-new-secret-0", time.Hour, false, nil, "old-secret-old-secret-old-secret-0").Holder(cookie.Value)
	require.True(t, ok)
	assert.EqualValues(t, 7, id)
	assert.EqualValues(t, 3, epoch)

	_, _, ok = auth.NewSessions("new-secret-new-secret-new-secret-0", time.Hour, false, nil).Holder(cookie.Value)
	assert.False(t, ok)
}
