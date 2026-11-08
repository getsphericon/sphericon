package oauth_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/oauthclient"
	"github.com/getsphericon/sphericon/ent/oauthcode"
	"github.com/getsphericon/sphericon/internal/api/oauth"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/identity/oauthclients"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func closedClient(t *testing.T) *ent.Client {
	t.Helper()
	sqlDB, err := sql.Open("pgx", "postgres://closed.invalid/none")
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	return entclient.New(sqlDB)
}

func errorOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
	return body["error"]
}

func TestAuthorizeRefusesOtherResponseTypesBackToTheClient(t *testing.T) {
	env := testhelper.Setup(t)

	rec := authorize(t, env, authorizeQuery(url.Values{"response_type": {"token"}}))

	require.Equal(t, http.StatusFound, rec.Code)
	loc, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, fixtureRedirectURI, loc.Scheme+"://"+loc.Host+loc.Path)
	assert.Equal(t, "invalid_request", loc.Query().Get("error"))
	assert.Equal(t, "response_type must be code", loc.Query().Get("error_description"))
	assert.Equal(t, "xyz", loc.Query().Get("state"))
	assert.Empty(t, loc.Query().Get("code"))
}

func TestTokenEndpointRefusesAMalformedBody(t *testing.T) {
	env := testhelper.Setup(t)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/oauth/token", strings.NewReader("code=%zz"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := serve(env, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "invalid_request", errorOf(t, rec))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
}

func TestAnExpiredCodeCannotBeExchanged(t *testing.T) {
	env := testhelper.Setup(t)
	code := consent(t, env, approval("contacts:read", false)).Query().Get("code")
	n, err := env.DB.OAuthCode.Update().SetExpiresAt(time.Now().Add(-time.Minute)).Save(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	rec := exchange(t, env, tokenForm(code))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "invalid_grant", errorOf(t, rec))
	assert.NotContains(t, rec.Body.String(), "access_token")
	used, err := env.DB.OAuthCode.Query().Where(oauthcode.UsedAtNotNil()).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, used, "an expired code is not consumed")
}

func TestRegistrationNamesAndBoundsClients(t *testing.T) {
	env := testhelper.Setup(t)
	register := func(body string) (*httptest.ResponseRecorder, string) {
		rec := postJSON(t, env, "/oauth/register", body)
		var reg map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &reg))
		id, _ := reg["client_id"].(string)
		return rec, id
	}

	rec, id := register(`{"client_name":"` + strings.Repeat("é", 150) + `","redirect_uris":["https://x.example/cb"]}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	stored, err := env.DB.OAuthClient.Query().Where(oauthclient.ClientID(id)).Only(t.Context())
	require.NoError(t, err)
	assert.Len(t, []rune(stored.Name), 100, "an overlong name is cut at 100 characters, not bytes")

	rec, id = register(`{"client_name":"   ","redirect_uris":["https://x.example/cb"]}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	stored, err = env.DB.OAuthClient.Query().Where(oauthclient.ClientID(id)).Only(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "MCP client", stored.Name, "a blank name gets the default")

	rec, _ = register(`{"redirect_uris":["http://127.0.0.1:8123/cb"]}`)
	assert.Equal(t, http.StatusCreated, rec.Code, "an IPv4 loopback http redirect is allowed")
	rec, _ = register(`{"redirect_uris":["http://[::1]:8123/cb"]}`)
	assert.Equal(t, http.StatusCreated, rec.Code, "an IPv6 loopback http redirect is allowed")

	tooMany := make([]string, 11)
	for i := range tooMany {
		tooMany[i] = `"https://x.example/cb` + string(rune('a'+i)) + `"`
	}
	for name, body := range map[string]string{
		"non-loopback ip over http": `{"redirect_uris":["http://10.0.0.5/cb"]}`,
		"other scheme with a host":  `{"redirect_uris":["ftp://x.example/cb"]}`,
		"credentials in the uri":    `{"redirect_uris":["https://user:pw@x.example/cb"]}`,
		"more than ten uris":        `{"redirect_uris":[` + strings.Join(tooMany, ",") + `]}`,
	} {
		rec, _ := register(body)
		assert.Equal(t, http.StatusBadRequest, rec.Code, name)
	}
}

// A storage outage is a server_error, never a client error or a silent success.
func TestStorageFailureIsAServerError(t *testing.T) {
	mux := http.NewServeMux()
	closed := closedClient(t)
	clients := oauthclients.New(closed, oauthclients.NewFetcher(webhook.Allowlist{}), time.Now)
	oauth.New(apitokens.New(closed, time.Now), clients, "https://app.test", "https://app.test").Mount(mux)
	do := func(req *http.Request) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	reg := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/oauth/register", strings.NewReader(`{"redirect_uris":["https://x.example/cb"]}`))
	rec := do(reg)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "server_error", errorOf(t, rec))

	rec = do(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/oauth/authorize?"+authorizeQuery(nil).Encode(), nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "server_error", errorOf(t, rec))

	form := tokenForm("whatever")
	tok := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	tok.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = do(tok)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "server_error", errorOf(t, rec))
	assert.NotContains(t, rec.Body.String(), "access_token")
}
