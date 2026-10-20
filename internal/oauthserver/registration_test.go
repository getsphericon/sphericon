package oauthserver_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/ent/oauthclient"
	"github.com/getsphericon/sphericon/internal/oauthserver"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// The fixture client (fixtures/oauth_clients.yml) and a fixed PKCE verifier.
const (
	fixtureClientID    = "fixture-client"
	fixtureRedirectURI = "https://connector.example/callback"
	verifier           = "0123456789012345678901234567890123456789012"
)

func postJSON(t *testing.T, env *testhelper.TestEnv, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return serve(env, req)
}

func TestDynamicClientRegistration(t *testing.T) {
	env := testhelper.Setup(t)

	rec := postJSON(t, env, "/oauth/register",
		`{"client_name":"Claude","redirect_uris":["https://claude.ai/api/mcp/auth_callback"],"token_endpoint_auth_method":"none"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var reg map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &reg))
	clientID, _ := reg["client_id"].(string)
	require.NotEmpty(t, clientID)
	assert.Equal(t, "Claude", reg["client_name"])
	assert.Equal(t, "none", reg["token_endpoint_auth_method"])
	assert.NotContains(t, reg, "client_secret", "public clients get no secret")

	stored, err := env.DB.OAuthClient.Query().Where(oauthclient.ClientID(clientID)).Only(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"https://claude.ai/api/mcp/auth_callback"}, stored.RedirectUris)
}

func TestDynamicClientRegistrationRejectsUnsafeMetadata(t *testing.T) {
	env := testhelper.Setup(t)

	for name, body := range map[string]string{
		"plain http redirect": `{"redirect_uris":["http://evil.example/cb"]}`,
		"fragment":            `{"redirect_uris":["https://x.example/cb#frag"]}`,
		"no redirect uris":    `{"client_name":"x"}`,
		"confidential client": `{"redirect_uris":["https://x.example/cb"],"token_endpoint_auth_method":"client_secret_basic"}`,
		"unsupported grant":   `{"redirect_uris":["https://x.example/cb"],"grant_types":["implicit"]}`,
		"javascript redirect": `{"redirect_uris":["javascript:alert(1)"]}`,
		"not json":            `nope`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := postJSON(t, env, "/oauth/register", body)
			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		})
	}

	rec := postJSON(t, env, "/oauth/register", `{"redirect_uris":["http://localhost:3000/cb"]}`)
	assert.Equal(t, http.StatusCreated, rec.Code, "loopback http redirect is allowed for native clients")
}

func authorizeQuery(extra url.Values) url.Values {
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {fixtureClientID},
		"redirect_uri":          {fixtureRedirectURI},
		"code_challenge":        {oauth2.S256ChallengeFromVerifier(verifier)},
		"code_challenge_method": {"S256"},
		"state":                 {"xyz"},
	}
	maps.Copy(q, extra)
	return q
}

func authorize(t *testing.T, env *testhelper.TestEnv, q url.Values) *httptest.ResponseRecorder {
	t.Helper()
	return serve(env, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/oauth/authorize?"+q.Encode(), nil))
}

func TestAuthorizeHandsOffToTheConsentScreen(t *testing.T) {
	env := testhelper.Setup(t)
	q := authorizeQuery(url.Values{"scope": {"contacts:read"}})

	rec := authorize(t, env, q)

	require.Equal(t, http.StatusFound, rec.Code)
	loc, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, appURL(t)+oauthserver.ConsentPath, loc.Scheme+"://"+loc.Host+loc.Path)
	assert.Equal(t, q, loc.Query(), "the SPA gets the original authorization request")
}

func TestAuthorizeNeverRedirectsToAnUnregisteredURI(t *testing.T) {
	env := testhelper.Setup(t)

	for name, extra := range map[string]url.Values{
		"unregistered redirect": {"redirect_uri": {"https://evil.example/cb"}},
		"unknown client":        {"client_id": {"nobody"}},
	} {
		t.Run(name, func(t *testing.T) {
			rec := authorize(t, env, authorizeQuery(extra))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Empty(t, rec.Header().Get("Location"))
		})
	}
}

func TestAuthorizeRequiresPKCE(t *testing.T) {
	env := testhelper.Setup(t)
	q := authorizeQuery(nil)
	q.Del("code_challenge")

	rec := authorize(t, env, q)

	require.Equal(t, http.StatusFound, rec.Code)
	loc, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, fixtureRedirectURI, loc.Scheme+"://"+loc.Host+loc.Path)
	assert.Equal(t, "invalid_request", loc.Query().Get("error"))
	assert.Equal(t, "xyz", loc.Query().Get("state"))
}
