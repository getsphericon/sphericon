package oauthauthorization_test

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

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/apitoken"
	"github.com/getsphericon/sphericon/ent/oauthrefreshtoken"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
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

// connectionOf is the Connection the access token was issued for.
func connectionOf(t *testing.T, env *testhelper.TestEnv, accessToken string) *ent.ApiToken {
	t.Helper()
	parsed := credentials.ParseToken(accessToken)
	require.NotNil(t, parsed, "an access token is an API token")
	row, err := env.DB.ApiToken.Query().Where(apitoken.Prefix(parsed.Prefix)).Only(t.Context())
	require.NoError(t, err)
	return row
}

// connection is the fixture Connection with the id, as it is stored now.
func connection(t *testing.T, env *testhelper.TestEnv, id int64) *ent.ApiToken {
	t.Helper()
	return env.DB.ApiToken.GetX(t.Context(), id)
}

// The access tokens of the fixture Connections (fixtures/api_tokens.yml), as the client was issued
// them; their refresh tokens are fixtures.VehementOwnerRefreshToken and VehementMemberRefreshToken.
var (
	valsConnection  = credentials.TokenValue(fixtures.VehementOwnerConnectionPrefix, fixtures.VehementOwnerConnectionSecret)
	adasConnection  = credentials.TokenValue(fixtures.VehementAdminConnectionPrefix, fixtures.VehementAdminConnectionSecret)
	melsConnection  = credentials.TokenValue(fixtures.VehementMemberConnectionPrefix, fixtures.VehementMemberConnectionSecret)
	ritasConnection = credentials.TokenValue(fixtures.UmbrellaRitaConnectionPrefix, fixtures.UmbrellaRitaConnectionSecret)
)

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
	return serve(env, mcpRequest(t, bearer)).Code
}

// mcpRequest is a call of /mcp with a bearer.
func mcpRequest(t *testing.T, bearer string) *http.Request {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, testhelper.MCPEndpoint, strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	return req
}

func TestCodeExchangeIssuesAnExpiringTokenAndARefreshToken(t *testing.T) {
	env := testhelper.Setup(t)

	tok := connect(t, env, "contacts:read")

	assert.Equal(t, 3600, tok.ExpiresIn)
	assert.NotEmpty(t, tok.RefreshToken)
	row := connectionOf(t, env, tok.AccessToken)
	require.NotNil(t, row.ExpiresAt)
	assert.WithinDuration(t, row.CreatedAt.Add(time.Hour), *row.ExpiresAt, time.Minute)
	refresh, err := env.DB.OAuthRefreshToken.Query().Where(oauthrefreshtoken.APITokenID(row.ID)).Only(t.Context())
	require.NoError(t, err)
	assert.Nil(t, refresh.UsedAt)
	assert.NotEqual(t, tok.RefreshToken, refresh.TokenHash, "only the hash is stored")
}

func TestAnExpiredAccessTokenIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	require.NotEqual(t, http.StatusUnauthorized, mcpStatus(t, env, valsConnection))

	row := connection(t, env, fixtures.VehementOwnerConnectionID)
	_, err := env.DB.ApiToken.UpdateOneID(row.ID).SetExpiresAt(row.CreatedAt.Add(-time.Minute)).Save(t.Context())
	require.NoError(t, err)

	assert.Equal(t, http.StatusUnauthorized, mcpStatus(t, env, valsConnection))
}

func TestRefreshRotatesTheTokenInPlace(t *testing.T) {
	env := testhelper.Setup(t)
	before := connection(t, env, fixtures.VehementOwnerConnectionID)

	rec := exchange(t, env, refreshForm(fixtures.VehementOwnerRefreshToken))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	second := decodeToken(t, rec)
	assert.Equal(t, 3600, second.ExpiresIn)
	assert.Equal(t, "contacts:read segments:read", second.Scope)
	assert.NotEqual(t, valsConnection, second.AccessToken)
	assert.NotEqual(t, fixtures.VehementOwnerRefreshToken, second.RefreshToken)

	// The Connection keeps its row; the old secret is dead and the new one works.
	assert.True(t, strings.HasPrefix(second.AccessToken, "omtk_"+before.Prefix+"_"), "the prefix stays")
	after := connection(t, env, fixtures.VehementOwnerConnectionID)
	assert.Nil(t, after.RevokedAt)
	assert.NotEqual(t, before.SecretHash, after.SecretHash)
	assert.Equal(t, http.StatusUnauthorized, mcpStatus(t, env, valsConnection))
	assert.NotEqual(t, http.StatusUnauthorized, mcpStatus(t, env, second.AccessToken))

	// The old refresh token is kept, used, so a replay is recognised.
	spent, err := env.DB.OAuthRefreshToken.Query().
		Where(oauthrefreshtoken.ID(fixtures.VehementOwnerRefreshID), oauthrefreshtoken.UsedAtNotNil()).Exist(t.Context())
	require.NoError(t, err)
	assert.True(t, spent)
}

func TestRefreshNarrowsTheGrantForGood(t *testing.T) {
	env := testhelper.Setup(t)
	form := refreshForm(fixtures.VehementOwnerRefreshToken)
	form.Set("scope", "segments:read")

	rec := exchange(t, env, form)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	narrowed := decodeToken(t, rec)
	assert.Equal(t, "segments:read", narrowed.Scope)
	assert.Equal(t, []string{"segments:read"}, connection(t, env, fixtures.VehementOwnerConnectionID).Scopes)

	// It cannot be widened back to the original scopes.
	wider := refreshForm(narrowed.RefreshToken)
	wider.Set("scope", "contacts:read segments:read")
	assert.Equal(t, "invalid_scope", decodeToken(t, exchange(t, env, wider)).Error)
}

func TestReusingARotatedRefreshTokenRevokesTheGrant(t *testing.T) {
	env := testhelper.Setup(t)
	rotated := decodeToken(t, exchange(t, env, refreshForm(fixtures.VehementOwnerRefreshToken)))

	replay := exchange(t, env, refreshForm(fixtures.VehementOwnerRefreshToken))

	assert.Equal(t, http.StatusBadRequest, replay.Code)
	assert.Equal(t, "invalid_grant", decodeToken(t, replay).Error)
	assert.NotNil(t, connection(t, env, fixtures.VehementOwnerConnectionID).RevokedAt)
	assert.Equal(t, http.StatusUnauthorized, mcpStatus(t, env, rotated.AccessToken))
	assert.Equal(t, http.StatusBadRequest, exchange(t, env, refreshForm(rotated.RefreshToken)).Code, "the newest refresh token died with the grant")
}

func TestRevokingTheConnectionKillsItsRefreshToken(t *testing.T) {
	env := testhelper.Setup(t)

	err := env.SiteActor(t, fixtures.VehementOwnerValEmail).SiteConnectionsDelete(t.Context(), siteapi.SiteConnectionsDeleteParams{
		WorkspaceId: fixtures.VehementPublicID,
		ID:          siteapi.EntityId(fixtures.VehementOwnerConnectionID),
	})
	require.NoError(t, err)

	assert.Equal(t, http.StatusUnauthorized, mcpStatus(t, env, valsConnection))
	rec := exchange(t, env, refreshForm(fixtures.VehementOwnerRefreshToken))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "invalid_grant", decodeToken(t, rec).Error)
}

func TestRefreshRefusals(t *testing.T) {
	env := testhelper.Setup(t)

	for name, tc := range map[string]struct {
		form url.Values
		want string
	}{
		"unknown token": {refreshForm("nope"), "invalid_grant"},
		"other client": {func() url.Values {
			f := refreshForm(fixtures.VehementOwnerRefreshToken)
			f.Set("client_id", "someone-else")
			return f
		}(), "invalid_grant"},
		"wider scope": {func() url.Values {
			f := refreshForm(fixtures.VehementOwnerRefreshToken)
			f.Set("scope", "contacts:read contacts:write")
			return f
		}(), "invalid_scope"},
		"foreign resource": {func() url.Values {
			f := refreshForm(fixtures.VehementOwnerRefreshToken)
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
	assert.Equal(t, http.StatusOK, exchange(t, env, refreshForm(fixtures.VehementOwnerRefreshToken)).Code)
}

func TestAnExpiredRefreshTokenIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	row := connection(t, env, fixtures.VehementOwnerConnectionID)
	_, err := env.DB.OAuthRefreshToken.UpdateOneID(fixtures.VehementOwnerRefreshID).
		SetExpiresAt(row.CreatedAt.Add(-time.Minute)).
		Save(t.Context())
	require.NoError(t, err)

	rec := exchange(t, env, refreshForm(fixtures.VehementOwnerRefreshToken))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "invalid_grant", decodeToken(t, rec).Error)
}
