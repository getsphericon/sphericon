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

	"github.com/getsphericon/sphericon/ent"
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

// connector is the Connection the fixture client's grant lives in.
func connector(t *testing.T, env *testhelper.TestEnv) *ent.ApiToken {
	t.Helper()
	row, err := env.DB.ApiToken.Query().Where(apitoken.Name("Fixture Connector"), apitoken.MembershipIDNotNil()).Only(t.Context())
	require.NoError(t, err)
	return row
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
	row := connector(t, env)
	require.NotNil(t, row.ExpiresAt)
	assert.WithinDuration(t, row.CreatedAt.Add(time.Hour), *row.ExpiresAt, time.Minute)
	refresh, err := env.DB.OAuthRefreshToken.Query().Where(oauthrefreshtoken.APITokenID(row.ID)).Only(t.Context())
	require.NoError(t, err)
	assert.Nil(t, refresh.UsedAt)
	assert.NotEqual(t, tok.RefreshToken, refresh.TokenHash, "only the hash is stored")
}

func TestAnExpiredAccessTokenIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	tok := connect(t, env, "contacts:read")
	require.NotEqual(t, http.StatusUnauthorized, mcpStatus(t, env, tok.AccessToken))

	row := connector(t, env)
	_, err := env.DB.ApiToken.UpdateOneID(row.ID).SetExpiresAt(row.CreatedAt.Add(-time.Minute)).Save(t.Context())
	require.NoError(t, err)

	assert.Equal(t, http.StatusUnauthorized, mcpStatus(t, env, tok.AccessToken))
}

func TestRefreshRotatesTheTokenInPlace(t *testing.T) {
	env := testhelper.Setup(t)
	first := connect(t, env, "contacts:read segments:read")
	before := connector(t, env)

	rec := exchange(t, env, refreshForm(first.RefreshToken))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	second := decodeToken(t, rec)
	assert.Equal(t, 3600, second.ExpiresIn)
	assert.Equal(t, "contacts:read segments:read", second.Scope)
	assert.NotEqual(t, first.AccessToken, second.AccessToken)
	assert.NotEqual(t, first.RefreshToken, second.RefreshToken)

	// The Connection keeps its row; the old secret is dead and the new one works.
	assert.True(t, strings.HasPrefix(second.AccessToken, "omtk_"+before.Prefix+"_"), "the prefix stays")
	after := connector(t, env)
	assert.Equal(t, before.ID, after.ID)
	assert.Nil(t, after.RevokedAt)
	assert.NotEqual(t, before.SecretHash, after.SecretHash)
	assert.Equal(t, http.StatusUnauthorized, mcpStatus(t, env, first.AccessToken))
	assert.NotEqual(t, http.StatusUnauthorized, mcpStatus(t, env, second.AccessToken))

	// The old refresh token is kept, used, so a replay is recognised.
	spent, err := env.DB.OAuthRefreshToken.Query().
		Where(oauthrefreshtoken.APITokenID(before.ID), oauthrefreshtoken.UsedAtNotNil()).Exist(t.Context())
	require.NoError(t, err)
	assert.True(t, spent)
}

func TestRefreshNarrowsTheGrantForGood(t *testing.T) {
	env := testhelper.Setup(t)
	first := connect(t, env, "contacts:read segments:read")
	form := refreshForm(first.RefreshToken)
	form.Set("scope", "segments:read")

	rec := exchange(t, env, form)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	narrowed := decodeToken(t, rec)
	assert.Equal(t, "segments:read", narrowed.Scope)
	assert.Equal(t, []string{"segments:read"}, connector(t, env).Scopes)

	// It cannot be widened back to the original scopes.
	wider := refreshForm(narrowed.RefreshToken)
	wider.Set("scope", "contacts:read segments:read")
	assert.Equal(t, "invalid_scope", decodeToken(t, exchange(t, env, wider)).Error)
}

func TestReusingARotatedRefreshTokenRevokesTheGrant(t *testing.T) {
	env := testhelper.Setup(t)
	first := connect(t, env, "contacts:read")
	rotated := decodeToken(t, exchange(t, env, refreshForm(first.RefreshToken)))

	replay := exchange(t, env, refreshForm(first.RefreshToken))

	assert.Equal(t, http.StatusBadRequest, replay.Code)
	assert.Equal(t, "invalid_grant", decodeToken(t, replay).Error)
	assert.NotNil(t, connector(t, env).RevokedAt)
	assert.Equal(t, http.StatusUnauthorized, mcpStatus(t, env, rotated.AccessToken))
	assert.Equal(t, http.StatusBadRequest, exchange(t, env, refreshForm(rotated.RefreshToken)).Code, "the newest refresh token died with the grant")
}

func TestRevokingTheConnectionKillsItsRefreshToken(t *testing.T) {
	env := testhelper.Setup(t)
	tok := connect(t, env, "contacts:read")
	row := connector(t, env)

	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteConnectionsDelete(t.Context(), siteapi.SiteConnectionsDeleteParams{
		WorkspaceId: fixtures.AcmePublicID,
		ID:          siteapi.EntityId(row.ID),
	})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteConnectionsDeleteNoContent{}, res)

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
	row := connector(t, env)
	_, err := env.DB.OAuthRefreshToken.Update().
		Where(oauthrefreshtoken.APITokenID(row.ID)).
		SetExpiresAt(row.CreatedAt.Add(-time.Minute)).
		Save(t.Context())
	require.NoError(t, err)

	rec := exchange(t, env, refreshForm(tok.RefreshToken))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "invalid_grant", decodeToken(t, rec).Error)
}
