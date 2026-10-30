package testhelper

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// RedirectBrowser is one browser talking to the server by plain requests (and the JSON posts of a
// page it landed on): it keeps cookies like a
// browser does and never follows a redirect on its own, so a test reads each Location and
// carries it to the next hop (the provider, then back), as the person's browser does.
type RedirectBrowser struct {
	env *TestEnv
	jar http.CookieJar
}

// Reply is what the server answered one GET, with its body already read.
type Reply struct {
	StatusCode int
	Header     http.Header
	Body       string
	cookies    []*http.Cookie
}

// Cookies are the cookies the response set.
func (r Reply) Cookies() []*http.Cookie { return r.cookies }

// NewRedirectBrowser is a browser with no cookies.
func (env *TestEnv) NewRedirectBrowser(t *testing.T) *RedirectBrowser {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	return &RedirectBrowser{env: env, jar: jar}
}

// Get sends a GET for a path (with its query) to the server, as this browser.
func (b *RedirectBrowser) Get(t *testing.T, target string) Reply {
	t.Helper()
	return b.send(t, http.MethodGet, target)
}

// Delete sends a DELETE for a path to the server, as this browser.
func (b *RedirectBrowser) Delete(t *testing.T, target string) Reply {
	t.Helper()
	return b.send(t, http.MethodDelete, target)
}

// PostJSON sends a POST with a JSON body to the server, as this browser: the SPA's request from a
// page the browser landed on.
func (b *RedirectBrowser) PostJSON(t *testing.T, target, body string) Reply {
	t.Helper()
	return b.sendBody(t, http.MethodPost, target, strings.NewReader(body))
}

func (b *RedirectBrowser) send(t *testing.T, method, target string) Reply {
	t.Helper()
	return b.sendBody(t, method, target, nil)
}

func (b *RedirectBrowser) sendBody(t *testing.T, method, target string, body io.Reader) Reply {
	t.Helper()
	u, err := url.Parse(target)
	require.NoError(t, err)
	cookieURL := url.URL{Scheme: "https", Host: "app.test", Path: u.Path}
	req := httptest.NewRequestWithContext(t.Context(), method, target, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range b.jar.Cookies(&cookieURL) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	b.env.Server.ServeHTTP(rec, req)
	resp := rec.Result()
	answer, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	b.jar.SetCookies(&cookieURL, resp.Cookies())
	return Reply{StatusCode: resp.StatusCode, Header: resp.Header, Body: string(answer), cookies: resp.Cookies()}
}
