//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"net/url"

	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// browserAgent is a person's browser, not Go's default agent: that one is machine engagement
// (ADR 0053).
const browserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15"

// openLink is a person opening a link from an email: a browser's GET, which must answer the
// redirect status, and the Location it is sent on to, resolved as a browser resolves it. The
// Location is not requested: it is a page of the SPA or a site of the sender, neither of which
// the suite serves.
func (w *Workspace) openLink(link string, status int) *url.URL {
	w.t.Helper()
	req, err := http.NewRequestWithContext(w.t.Context(), http.MethodGet, link, http.NoBody)
	require.NoError(w.t, err)
	req.Header.Set("User-Agent", browserAgent)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	require.NoError(w.t, err)
	_, err = io.Copy(io.Discard, res.Body)
	require.NoError(w.t, err)
	require.NoError(w.t, res.Body.Close())
	require.Equal(w.t, status, res.StatusCode, "GET %s", link)
	location, err := res.Location()
	require.NoError(w.t, err, "GET %s names where it goes", link)
	return location
}

// pageToken is the token a page of the app reads from its own address: the page must be the
// app's, at path, and carry a token.
func (w *Workspace) pageToken(page *url.URL, path string) string {
	w.t.Helper()
	app, err := url.Parse(w.env.BaseURL)
	require.NoError(w.t, err)
	appOrigin := app.Scheme + "://" + app.Host
	pageOrigin := page.Scheme + "://" + page.Host
	require.Equal(w.t, appOrigin, pageOrigin, "the page %s is on the app", page)
	require.Equal(w.t, path, page.Path, "the link lands on the %s page", path)
	token := page.Query().Get("token")
	require.NotEmpty(w.t, token, "the page %s carries a token", page)
	return token
}

// anonymousSite is the SPA's API as a visitor who is not signed in calls it: the recipient of an
// email, on a page reached from a link in it.
func (w *Workspace) anonymousSite() *siteapi.Client {
	w.t.Helper()
	site, err := siteapi.NewClient(w.env.BaseURL+"/site", cookieSource(""))
	require.NoError(w.t, err)
	return site
}
