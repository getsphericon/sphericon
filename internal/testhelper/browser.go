package testhelper

import (
	"net/http"
	"net/http/cookiejar"
	"testing"

	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// browserSiteBase is the /site of a SiteBrowser: the in-memory transport has no TLS, but the
// instance sets Secure cookies as it does behind its HTTPS edge, and a jar withholds those from a
// plain-http URL.
const browserSiteBase = "https://local/site"

// SiteBrowser returns an anonymous /site client that keeps cookies between its requests
// the way one browser does: a cookie a response sets goes back on the next request whose
// URL its path and attributes match. Two SiteBrowser clients are two browsers, which is
// what the Sign-up browser tests need (ADR 0030). Like the other actors it follows no
// redirect: a test reads the answer it got.
func (env *TestEnv) SiteBrowser(t *testing.T) *siteapi.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	browser := &http.Client{
		Transport:     handlerTripper{handler: env.Server},
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	c, err := siteapi.NewClient(browserSiteBase, cookieSource{""}, siteapi.WithClient(browser))
	require.NoError(t, err)
	return c
}
