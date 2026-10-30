package site_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const loginPath = "/site/auth/login"

// loginBody is the JSON body of the site login operation.
func loginBody(email, password string) string {
	return fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
}

// postLogin drives the real login endpoint through the whole server (cookies and
// headers are only observable on the wire).
func postLogin(t *testing.T, env *testhelper.TestEnv, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, loginPath, strings.NewReader(loginBody(email, password)))
	req.Header.Set("Content-Type", "application/json")
	env.Server.ServeHTTP(rec, req)
	return rec
}

// sessionCookie returns the "JWT" cookie set on the response, or nil.
func sessionCookie(resp *http.Response) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == "JWT" {
			return c
		}
	}
	return nil
}

func TestSiteLoginStartsASessionThatAuthorizes(t *testing.T) {
	cfg, err := config.Load("test")
	require.NoError(t, err)
	env := testhelper.Setup(t)

	rec := postLogin(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.JSONEq(t, `{"outcome":"session"}`, rec.Body.String())

	cookie := sessionCookie(rec.Result())
	require.NotNil(t, cookie, "login sets the session cookie")
	assert.NotEmpty(t, cookie.Value)
	assert.Equal(t, "/", cookie.Path)
	assert.True(t, cookie.HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	assert.Equal(t, strings.HasPrefix(cfg.AppURL, "https://"), cookie.Secure, "Secure whenever the instance is served over HTTPS")
	assert.Equal(t, int(cfg.SessionTTL.Seconds()), cookie.MaxAge, "the cookie lives as long as the session")

	assert.Equal(t, http.StatusOK, workspacesStatus(t, env, cookie.Value), "the cookie authorizes /site")
}

// Unknown email and wrong password must not be told apart: same status, same body,
// same headers, and no cookie either way.
func TestSiteLoginAnswersUnknownEmailAndWrongPasswordIdentically(t *testing.T) {
	env := testhelper.Setup(t)

	wrong := postLogin(t, env, fixtures.OwnerJohnEmail, "wrong-password")
	unknown := postLogin(t, env, "nobody@nowhere.test", "wrong-password")
	malformed := postLogin(t, env, "not-an-email", "wrong-password")

	require.Equal(t, http.StatusUnauthorized, wrong.Code, wrong.Body.String())
	for name, rec := range map[string]*httptest.ResponseRecorder{"unknown": unknown, "malformed": malformed} {
		assert.Equal(t, wrong.Code, rec.Code, name)
		assert.Equal(t, wrong.Body.String(), rec.Body.String(), name)
		w, u := wrong.Header().Clone(), rec.Header().Clone()
		w.Del("X-Request-ID")
		u.Del("X-Request-ID")
		assert.Equal(t, w, u, name)
		assert.Empty(t, rec.Result().Cookies(), name)
	}
	assert.Empty(t, wrong.Result().Cookies())
}

func TestSiteLoginThroughTheGeneratedClient(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)

	res, err := c.SiteAuthLogin(context.Background(), &siteapi.SiteLoginInput{Email: fixtures.OwnerJohnEmail, Password: fixtures.OwnerJohnPassword})
	require.NoError(t, err)
	ok, isOK := res.(*siteapi.SiteLoginResultHeaders)
	require.Truef(t, isOK, "got %T", res)
	assert.Equal(t, siteapi.SiteLoginOutcomeSession, ok.Response.Outcome)
	assert.True(t, ok.SetCookie.Set)

	res, err = c.SiteAuthLogin(context.Background(), &siteapi.SiteLoginInput{Email: fixtures.OwnerJohnEmail, Password: "wrong"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAuthLoginUnauthorized{}, res)
}

func TestSiteLogoutClearsTheSessionCookie(t *testing.T) {
	env := testhelper.Setup(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/site/auth/logout", nil)
	env.Server.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	cookie := sessionCookie(rec.Result())
	require.NotNil(t, cookie, "logout overwrites the session cookie")
	assert.Empty(t, cookie.Value)
	assert.Negative(t, cookie.MaxAge, "the cookie is expired")
	assert.Equal(t, "/", cookie.Path)
}

// The only route that mints a session is the site login: the library's own
// routes are gone, and they must not fall through to the SPA shell either.
func TestTheLibraryAuthAndAvatarRoutesAreGone(t *testing.T) {
	env := testhelper.Setup(t)
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/auth/direct/login"},
		{http.MethodGet, "/auth/direct/login?user=a&passwd=b"},
		{http.MethodGet, "/auth/logout"},
		{http.MethodGet, "/auth/list"},
		{http.MethodGet, "/avatar/whatever.image"},
	} {
		rec := httptest.NewRecorder()
		body := strings.NewReader(loginBody(fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword))
		req := httptest.NewRequestWithContext(t.Context(), c.method, c.path, body)
		req.Header.Set("Content-Type", "application/json")
		env.Server.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code, c.path)
		assert.Empty(t, rec.Result().Cookies(), c.path)
	}
}

func TestSiteLoginRefusesAUserWithoutAPassword(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, env.DB.User.UpdateOneID(fixtures.OwnerJaneID).SetPasswordHash("").Exec(t.Context()))

	rec := postLogin(t, env, fixtures.OwnerJaneEmail, "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "even with an empty password")
	assert.Nil(t, sessionCookie(rec.Result()))
}

// TestSiteWorkspacesRequireAuth brackets the guard: no cookie => 401.
func TestSiteWorkspacesRequireAuth(t *testing.T) {
	env := testhelper.Setup(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/site/workspaces", nil)
	env.Server.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
