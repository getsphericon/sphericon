package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// loginWithFetchSite posts the fixture owner's credentials through the whole server,
// with an optional Sec-Fetch-Site (actors cannot set it or expose the cookies).
func loginWithFetchSite(t *testing.T, env *testhelper.TestEnv, fetchSite string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"email":"` + fixtures.OwnerJohnEmail + `","password":"` + fixtures.OwnerJohnPassword + `"}`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/site/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if fetchSite != "" {
		req.Header.Set("Sec-Fetch-Site", fetchSite)
	}
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	return rec
}

func TestSessionCookieIsSameSiteLax(t *testing.T) {
	env := testhelper.Setup(t)

	rec := loginWithFetchSite(t, env, "same-origin")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var jwt *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "JWT" {
			jwt = c
		}
	}
	require.NotNil(t, jwt, "login sets the JWT cookie")
	assert.Equal(t, http.SameSiteLaxMode, jwt.SameSite)
	assert.True(t, jwt.HttpOnly)
}

func TestCrossSiteLoginIsRejected(t *testing.T) {
	env := testhelper.Setup(t)

	rec := loginWithFetchSite(t, env, "cross-site")
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, rec.Result().Cookies())
}
