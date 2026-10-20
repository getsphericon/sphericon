package testhelper

import (
	"net/http"
	"net/http/cookiejar"
	"testing"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	ht "github.com/ogen-go/ogen/http"
	"github.com/stretchr/testify/require"
)

// SiteBrowser returns an anonymous /site client that keeps cookies between its requests
// the way one browser does: a cookie a response sets goes back on the next request whose
// URL its path and attributes match. Two SiteBrowser clients are two browsers, which is
// what the Registration browser tests need (ADR 0030).
func (env *TestEnv) SiteBrowser(t *testing.T) *siteapi.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	c, err := siteapi.NewClient(siteBase, cookieSource{""}, siteapi.WithClient(jarClient{inner: env.Transport(nil), jar: jar}))
	require.NoError(t, err)
	return c
}

type jarClient struct {
	inner ht.Client
	jar   http.CookieJar
}

// Do keeps the jar on the origin's https form: the in-memory transport has no TLS, but the
// instance sets Secure cookies as it does behind its HTTPS edge, and a jar withholds those
// from a plain-http URL.
func (c jarClient) Do(r *http.Request) (*http.Response, error) {
	origin := *r.URL
	origin.Scheme = "https"
	for _, cookie := range c.jar.Cookies(&origin) {
		r.AddCookie(cookie)
	}
	resp, err := c.inner.Do(r)
	if err != nil {
		return nil, err
	}
	c.jar.SetCookies(&origin, resp.Cookies())
	return resp, nil
}
