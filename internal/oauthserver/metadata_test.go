package oauthserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/getsphericon/sphericon/ent/apitoken"
	"github.com/getsphericon/sphericon/ent/oauthclient"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const metadataRedirectURI = "https://claude.example/callback"

// metadataServer serves a Client ID Metadata Document over TLS. document receives the
// client_id URL and returns the body; the server counts the fetches.
type metadataServer struct {
	*httptest.Server
	ClientID string
	fetches  atomic.Int32
}

func newMetadataServer(t *testing.T, cacheControl string, document func(clientID string) string) *metadataServer {
	t.Helper()
	m := &metadataServer{}
	m.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		m.fetches.Add(1)
		if cacheControl != "" {
			w.Header().Set("Cache-Control", cacheControl)
		}
		_, _ = w.Write([]byte(document(m.ClientID)))
	}))
	t.Cleanup(m.Close)
	m.ClientID = m.URL + "/client.json"
	return m
}

func goodDocument(clientID string) string {
	body, _ := json.Marshal(map[string]any{
		"client_id": clientID, "client_name": "Claude", "redirect_uris": []string{metadataRedirectURI},
	})
	return string(body)
}

func setupWithFetcher(t *testing.T, m *metadataServer) *testhelper.TestEnv {
	t.Helper()
	return testhelper.Setup(t, testhelper.WithOAuthFetcher(m.Client()))
}

func metadataAuthorize(t *testing.T, env *testhelper.TestEnv, clientID, redirectURI string) *httptest.ResponseRecorder {
	t.Helper()
	return authorize(t, env, authorizeQuery(url.Values{"client_id": {clientID}, "redirect_uri": {redirectURI}}))
}

func TestAuthorizationServerAdvertisesMetadataDocumentSupport(t *testing.T) {
	env := testhelper.Setup(t)

	as := getJSON(t, env, "/.well-known/oauth-authorization-server")

	assert.Equal(t, true, as["client_id_metadata_document_supported"])
	assert.NotEmpty(t, as["registration_endpoint"], "dynamic client registration keeps working")
}

func TestMetadataDocumentClientCompletesTheAuthorizationCodeFlow(t *testing.T) {
	m := newMetadataServer(t, "", goodDocument)
	env := setupWithFetcher(t, m)

	rec := metadataAuthorize(t, env, m.ClientID, metadataRedirectURI)
	require.Equal(t, http.StatusFound, rec.Code, rec.Body.String())

	decision := approval("contacts:read", false)
	decision.ClientId, decision.RedirectUri = m.ClientID, metadataRedirectURI
	code := consent(t, env, decision).Query().Get("code")
	require.NotEmpty(t, code)

	form := tokenForm(code)
	form.Set("client_id", m.ClientID)
	form.Set("redirect_uri", metadataRedirectURI)
	tok := exchange(t, env, form)
	require.Equal(t, http.StatusOK, tok.Code, tok.Body.String())

	stored, err := env.DB.OAuthClient.Query().Where(oauthclient.ClientID(m.ClientID)).Only(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "Claude", stored.Name)
	assert.Equal(t, []string{metadataRedirectURI}, stored.RedirectUris)
	assert.NotNil(t, stored.MetadataExpiresAt)
	minted, err := env.DB.Scoped(fixtures.AcmeID).ApiToken().Query().Where(apitoken.Name("Claude (MCP)")).Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, minted)
}

func TestMetadataDocumentClientIsRefusedWhenTheDocumentIsWrong(t *testing.T) {
	for name, doc := range map[string]func(string) string{
		"client_id differs from the URL": func(string) string {
			return goodDocument("https://other.example/client.json")
		},
		"no redirect uris": func(id string) string { return `{"client_id":"` + id + `"}` },
		"unsafe redirect uri": func(id string) string {
			return `{"client_id":"` + id + `","redirect_uris":["http://evil.example/cb"]}`
		},
		"confidential client": func(id string) string {
			return `{"client_id":"` + id + `","redirect_uris":["` + metadataRedirectURI + `"],"token_endpoint_auth_method":"client_secret_basic"}`
		},
		"not json": func(string) string { return "nope" },
		"too large": func(id string) string {
			return `{"client_id":"` + id + `","client_name":"` + strings.Repeat("x", 20<<10) + `"}`
		},
	} {
		t.Run(name, func(t *testing.T) {
			m := newMetadataServer(t, "", doc)
			env := setupWithFetcher(t, m)

			rec := metadataAuthorize(t, env, m.ClientID, metadataRedirectURI)

			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Empty(t, rec.Header().Get("Location"))
			n, err := env.DB.OAuthClient.Query().Where(oauthclient.ClientID(m.ClientID)).Count(t.Context())
			require.NoError(t, err)
			assert.Zero(t, n, "a refused document is not stored")
		})
	}
}

func TestMetadataDocumentClientIsRefusedForAnUnlistedRedirectURI(t *testing.T) {
	m := newMetadataServer(t, "", goodDocument)
	env := setupWithFetcher(t, m)

	rec := metadataAuthorize(t, env, m.ClientID, "https://evil.example/cb")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, rec.Header().Get("Location"))
}

func TestMetadataDocumentClientIsRefusedForUnsafeURLs(t *testing.T) {
	m := newMetadataServer(t, "", goodDocument)
	// The production fetcher: the document lives on a loopback address.
	env := testhelper.Setup(t)

	for name, clientID := range map[string]string{
		"private address":  m.ClientID,
		"plain http":       strings.Replace(m.ClientID, "https://", "http://", 1),
		"no path":          m.URL,
		"fragment":         m.ClientID + "#frag",
		"embedded user":    strings.Replace(m.ClientID, "https://", "https://user:pw@", 1),
		"unreachable host": "https://metadata.invalid/client.json",
	} {
		t.Run(name, func(t *testing.T) {
			rec := metadataAuthorize(t, env, clientID, metadataRedirectURI)

			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Empty(t, rec.Header().Get("Location"))
		})
	}
	assert.Zero(t, m.fetches.Load(), "the SSRF guard stops the call before it reaches the server")
}

func TestMetadataDocumentIsCachedByHTTPHeaders(t *testing.T) {
	for name, tc := range map[string]struct {
		cacheControl string
		fetches      int32
	}{
		"max-age":  {"max-age=3600", 1},
		"default":  {"", 1},
		"no-store": {"no-store", 2},
	} {
		t.Run(name, func(t *testing.T) {
			m := newMetadataServer(t, tc.cacheControl, goodDocument)
			env := setupWithFetcher(t, m)

			for range 2 {
				rec := metadataAuthorize(t, env, m.ClientID, metadataRedirectURI)
				require.Equal(t, http.StatusFound, rec.Code, rec.Body.String())
			}

			assert.Equal(t, tc.fetches, m.fetches.Load(), "fetches with Cache-Control %q", tc.cacheControl)
		})
	}
}

func TestMetadataDocumentIsFetchedAgainOnceStale(t *testing.T) {
	var renamed atomic.Bool
	m := newMetadataServer(t, "max-age=0", func(id string) string {
		name := "Claude"
		if renamed.Load() {
			name = "Claude v2"
		}
		body, _ := json.Marshal(map[string]any{"client_id": id, "client_name": name, "redirect_uris": []string{metadataRedirectURI}})
		return string(body)
	})
	env := setupWithFetcher(t, m)
	require.Equal(t, http.StatusFound, metadataAuthorize(t, env, m.ClientID, metadataRedirectURI).Code)

	renamed.Store(true)
	require.Equal(t, http.StatusFound, metadataAuthorize(t, env, m.ClientID, metadataRedirectURI).Code)

	rows, err := env.DB.OAuthClient.Query().Where(oauthclient.ClientID(m.ClientID)).All(t.Context())
	require.NoError(t, err)
	require.Len(t, rows, 1, "the refresh updates the row")
	assert.Equal(t, "Claude v2", rows[0].Name)
}

func TestConsentScreenNamesTheMetadataClientHostAndWarnsAboutLocalhost(t *testing.T) {
	doc := func(id string) string {
		return `{"client_id":"` + id + `","client_name":"Local tool","redirect_uris":["http://localhost:3000/cb","` + metadataRedirectURI + `"]}`
	}
	m := newMetadataServer(t, "", doc)
	env := setupWithFetcher(t, m)
	host, err := url.Parse(m.ClientID)
	require.NoError(t, err)

	for redirect, local := range map[string]bool{"http://localhost:3000/cb": true, metadataRedirectURI: false} {
		res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteOAuthDescribe(t.Context(), siteapi.SiteOAuthDescribeParams{
			ClientId: m.ClientID, RedirectUri: redirect,
		})
		require.NoError(t, err)
		got, ok := res.(*siteapi.SiteOAuthAuthorizationRequest)
		require.Truef(t, ok, "got %T", res)
		assert.Equal(t, "Local tool", got.ClientName)
		assert.Equal(t, siteapi.NewOptString(host.Host), got.ClientHost)
		assert.Equal(t, local, got.LocalRedirect, redirect)
	}

	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteOAuthDescribe(t.Context(), siteapi.SiteOAuthDescribeParams{
		ClientId: fixtureClientID, RedirectUri: fixtureRedirectURI,
	})
	require.NoError(t, err)
	registered, ok := res.(*siteapi.SiteOAuthAuthorizationRequest)
	require.Truef(t, ok, "got %T", res)
	assert.False(t, registered.ClientHost.Set, "a registered client has no host to show")
	assert.False(t, registered.LocalRedirect)
}
