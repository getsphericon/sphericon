package site_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	apiauth "github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// workspacesStatus answers the status of a protected site request carrying the
// session cookie.
func workspacesStatus(t *testing.T, env *testhelper.TestEnv, session string) int {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/site/workspaces", nil)
	req.AddCookie(testhelper.RequestCookie("JWT", session))
	rec := env.Serve(req)
	return rec.Code
}

func TestSiteSessionEndsAfterTheConfiguredLifetime(t *testing.T) {
	cfg, err := config.Load(config.EnvTest)
	require.NoError(t, err)
	require.Equal(t, 24*time.Hour, cfg.SessionTTL, "default session lifetime")
	c := testhelper.NewClock(time.Now())
	env := testhelper.Setup(t, testhelper.WithClock(c.Now))

	rec := postLogin(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	cookie, hasCookie := lo.Find(rec.Result().Cookies(), func(ck *http.Cookie) bool { return ck.Name == "JWT" })
	require.True(t, hasCookie)
	assert.Equal(t, int(cfg.SessionTTL.Seconds()), cookie.MaxAge, "the cookie lives as long as the session")

	c.Advance(cfg.SessionTTL - time.Minute)
	assert.Equal(t, http.StatusOK, workspacesStatus(t, env, cookie.Value), "still inside the lifetime")

	c.Advance(2 * time.Minute)
	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, cookie.Value), "past the lifetime")
}

func TestSiteRejectsAnExpiredSessionToken(t *testing.T) {
	env := testhelper.Setup(t)
	expired := env.SiteToken(t, fixtures.OwnerJohnEmail, func(c *apiauth.SessionClaims) {
		c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
	})
	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, expired))
}

func TestSiteRejectsASessionTokenWithoutAnEpoch(t *testing.T) {
	env := testhelper.Setup(t)
	preEpoch := env.SiteToken(t, fixtures.OwnerJohnEmail, func(c *apiauth.SessionClaims) {
		c.Epoch = nil
	})
	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, preEpoch))
}

func TestSiteBumpedEpochEndsTheUsersExistingSessions(t *testing.T) {
	env := testhelper.Setup(t)
	old := env.SiteToken(t, fixtures.OwnerJohnEmail, nil)
	require.Equal(t, http.StatusOK, workspacesStatus(t, env, old))

	require.NoError(t, env.DB.User.UpdateOneID(fixtures.OwnerJohnID).AddSessionEpoch(1).Exec(t.Context()))

	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, old), "minted under the previous epoch")
	assert.Equal(t, http.StatusOK, workspacesStatus(t, env, env.SiteToken(t, fixtures.OwnerJohnEmail, nil)), "minted under the current epoch")
}

func TestSiteRejectsASessionTokenWithoutAMethod(t *testing.T) {
	env := testhelper.Setup(t)
	for name, edit := range map[string]func(*apiauth.SessionClaims){
		"no method":      func(c *apiauth.SessionClaims) { c.Method = "" },
		"unknown method": func(c *apiauth.SessionClaims) { c.Method = "telepathy" },
	} {
		assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, env.SiteToken(t, fixtures.OwnerJohnEmail, edit)), name)
	}
}
