package oauthserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/apitoken"
	"github.com/getsphericon/sphericon/ent/oauthrefreshtoken"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
}

func decodeToken(t *testing.T, rec *httptest.ResponseRecorder) tokenResponse {
	t.Helper()
	var tok tokenResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tok), rec.Body.String())
	return tok
}

// connect plays the whole consent and code exchange and returns the first token response.
func connect(t *testing.T, env *testhelper.TestEnv, scope string) tokenResponse {
	t.Helper()
	back := consent(t, env, approval(scope, false))
	rec := exchange(t, env, tokenForm(back.Query().Get("code")))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decodeToken(t, rec)
}

func refreshForm(refreshToken string) url.Values {
	return url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {fixtureClientID},
	}
}

// mcpStatus calls /mcp with a bearer and returns the HTTP status.
func mcpStatus(t *testing.T, env *testhelper.TestEnv, bearer string) int {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, testhelper.MCPEndpoint, strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	return serve(env, req).Code
}

func TestCodeExchangeIssuesAnExpiringTokenAndARefreshToken(t *testing.T) {
	env := testhelper.Setup(t)

	tok := connect(t, env, "contacts:read")

	assert.Equal(t, 3600, tok.ExpiresIn)
	assert.NotEmpty(t, tok.RefreshToken)
	row, err := env.DB.ApiToken.Query().Where(apitoken.Name("Fixture Connector (MCP)")).Only(t.Context())
	require.NoError(t, err)
	require.NotNil(t, row.ExpiresAt)
	assert.WithinDuration(t, time.Now().Add(time.Hour), *row.ExpiresAt, time.Minute)
	refresh, err := env.DB.OAuthRefreshToken.Query().Only(t.Context())
	require.NoError(t, err)
	assert.Equal(t, row.ID, refresh.APITokenID)
	assert.Nil(t, refresh.UsedAt)
	assert.NotEqual(t, tok.RefreshToken, refresh.TokenHash, "only the hash is stored")
}

func TestAnExpiredAccessTokenIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	tok := connect(t, env, "contacts:read")
	require.NotEqual(t, http.StatusUnauthorized, mcpStatus(t, env, tok.AccessToken))

	_, err := env.DB.ApiToken.Update().Where(apitoken.Name("Fixture Connector (MCP)")).SetExpiresAt(time.Now().Add(-time.Minute)).Save(t.Context())
	require.NoError(t, err)

	assert.Equal(t, http.StatusUnauthorized, mcpStatus(t, env, tok.AccessToken))
}

func TestRefreshRotatesTheTokenInPlace(t *testing.T) {
	env := testhelper.Setup(t)
	first := connect(t, env, "contacts:read segments:read")

	rec := exchange(t, env, refreshForm(first.RefreshToken))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	second := decodeToken(t, rec)
	assert.Equal(t, 3600, second.ExpiresIn)
	assert.Equal(t, "contacts:read segments:read", second.Scope)
	assert.NotEqual(t, first.AccessToken, second.AccessToken)
	assert.NotEqual(t, first.RefreshToken, second.RefreshToken)

	// One connector row stays in Settings; the old secret is dead and the new one works.
	rows, err := env.DB.ApiToken.Query().Where(apitoken.Name("Fixture Connector (MCP)")).All(t.Context())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Nil(t, rows[0].RevokedAt)
	assert.Equal(t, http.StatusUnauthorized, mcpStatus(t, env, first.AccessToken))
	assert.NotEqual(t, http.StatusUnauthorized, mcpStatus(t, env, second.AccessToken))

	// The old refresh token is kept, used, so a replay is recognised.
	used, err := env.DB.OAuthRefreshToken.Query().Where(oauthrefreshtoken.UsedAtNotNil()).Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, used)
}

func TestReusingARotatedRefreshTokenRevokesTheGrant(t *testing.T) {
	env := testhelper.Setup(t)
	first := connect(t, env, "contacts:read")
	rotated := decodeToken(t, exchange(t, env, refreshForm(first.RefreshToken)))

	replay := exchange(t, env, refreshForm(first.RefreshToken))

	assert.Equal(t, http.StatusBadRequest, replay.Code)
	assert.Equal(t, "invalid_grant", decodeToken(t, replay).Error)
	row, err := env.DB.ApiToken.Query().Where(apitoken.Name("Fixture Connector (MCP)")).Only(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, row.RevokedAt)
	assert.Equal(t, http.StatusUnauthorized, mcpStatus(t, env, rotated.AccessToken))
	assert.Equal(t, http.StatusBadRequest, exchange(t, env, refreshForm(rotated.RefreshToken)).Code, "the newest refresh token died with the grant")
}

func TestRevokingTheConnectorInSettingsKillsItsRefreshToken(t *testing.T) {
	env := testhelper.Setup(t)
	tok := connect(t, env, "contacts:read")
	row, err := env.DB.ApiToken.Query().Where(apitoken.Name("Fixture Connector (MCP)")).Only(t.Context())
	require.NoError(t, err)

	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteTokensDelete(t.Context(), siteapi.SiteTokensDeleteParams{
		Slug: fixtures.AcmeSlug,
		ID:   siteapi.EntityId(row.ID),
	})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteTokensDeleteNoContent{}, res)

	assert.Equal(t, http.StatusUnauthorized, mcpStatus(t, env, tok.AccessToken))
	rec := exchange(t, env, refreshForm(tok.RefreshToken))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "invalid_grant", decodeToken(t, rec).Error)
}

func TestRefreshRefusals(t *testing.T) {
	env := testhelper.Setup(t)
	tok := connect(t, env, "contacts:read")

	for name, tc := range map[string]struct {
		form url.Values
		want string
	}{
		"unknown token": {refreshForm("nope"), "invalid_grant"},
		"other client": {func() url.Values {
			f := refreshForm(tok.RefreshToken)
			f.Set("client_id", "someone-else")
			return f
		}(), "invalid_grant"},
		"wider scope": {func() url.Values {
			f := refreshForm(tok.RefreshToken)
			f.Set("scope", "contacts:read contacts:write")
			return f
		}(), "invalid_scope"},
		"foreign resource": {func() url.Values {
			f := refreshForm(tok.RefreshToken)
			f.Set("resource", "https://elsewhere.example/mcp")
			return f
		}(), "invalid_target"},
	} {
		t.Run(name, func(t *testing.T) {
			rec := exchange(t, env, tc.form)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, tc.want, decodeToken(t, rec).Error)
		})
	}

	// None of the refusals spent the token.
	assert.Equal(t, http.StatusOK, exchange(t, env, refreshForm(tok.RefreshToken)).Code)
}

func TestAnExpiredRefreshTokenIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	tok := connect(t, env, "contacts:read")
	_, err := env.DB.OAuthRefreshToken.Update().SetExpiresAt(time.Now().Add(-time.Minute)).Save(t.Context())
	require.NoError(t, err)

	rec := exchange(t, env, refreshForm(tok.RefreshToken))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "invalid_grant", decodeToken(t, rec).Error)
}
