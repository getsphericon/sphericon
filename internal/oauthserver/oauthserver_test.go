package oauthserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serve drives the real in-memory server (no sockets).
func serve(env *testhelper.TestEnv, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	return rec
}

func appURL(t *testing.T) string {
	t.Helper()
	cfg, err := config.Load("test")
	require.NoError(t, err)
	return cfg.AppURL
}

func getJSON(t *testing.T, env *testhelper.TestEnv, path string) map[string]any {
	t.Helper()
	rec := serve(env, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var doc map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &doc))
	return doc
}

const splitMCPURL = "https://mcp.sphericon.example"

func TestMCPChallengePointsAtProtectedResourceMetadata(t *testing.T) {
	env := testhelper.Setup(t)

	for name, auth := range map[string]string{
		"no token":      "",
		"unknown token": "Bearer omtk_deadbeef_invalidsecret",
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, testhelper.MCPEndpoint, strings.NewReader(`{}`))
			if auth != "" {
				req.Header.Set("Authorization", auth)
			}
			rec := serve(env, req)

			assert.Equal(t, http.StatusUnauthorized, rec.Code)
			assert.Contains(t, rec.Header().Get("WWW-Authenticate"),
				`resource_metadata="`+appURL(t)+`/.well-known/oauth-protected-resource/mcp"`)
		})
	}
}

func TestDiscoveryDocuments(t *testing.T) {
	env := testhelper.Setup(t)
	base := appURL(t)

	for _, path := range []string{
		"/.well-known/oauth-protected-resource/mcp",
		"/.well-known/oauth-protected-resource",
	} {
		prm := getJSON(t, env, path)
		assert.Equal(t, base+"/mcp", prm["resource"], path)
		assert.Equal(t, []any{base}, prm["authorization_servers"], path)
	}

	as := getJSON(t, env, "/.well-known/oauth-authorization-server")
	assert.Equal(t, base, as["issuer"])
	assert.Equal(t, base+"/oauth/authorize", as["authorization_endpoint"])
	assert.Equal(t, base+"/oauth/token", as["token_endpoint"])
	assert.Equal(t, base+"/oauth/register", as["registration_endpoint"])
	assert.Equal(t, []any{"code"}, as["response_types_supported"])
	assert.Equal(t, []any{"authorization_code", "refresh_token"}, as["grant_types_supported"])
	assert.Equal(t, []any{"S256"}, as["code_challenge_methods_supported"])
	assert.Equal(t, []any{"none"}, as["token_endpoint_auth_methods_supported"])
}

func TestSplitHostDiscovery(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.MCPURL = splitMCPURL + "/" }))
	issuer := appURL(t)

	for _, path := range []string{
		"/.well-known/oauth-protected-resource/mcp",
		"/.well-known/oauth-protected-resource",
	} {
		prm := getJSON(t, env, path)
		assert.Equal(t, splitMCPURL+"/mcp", prm["resource"], path)
		assert.Equal(t, []any{issuer}, prm["authorization_servers"], path)
	}

	as := getJSON(t, env, "/.well-known/oauth-authorization-server")
	assert.Equal(t, issuer, as["issuer"])
	assert.Equal(t, issuer+"/oauth/authorize", as["authorization_endpoint"])
	assert.Equal(t, issuer+"/oauth/token", as["token_endpoint"])
}

func TestSplitHostChallengeNamesTheMCPHost(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.MCPURL = splitMCPURL }))

	rec := serve(env, httptest.NewRequestWithContext(t.Context(), http.MethodPost, testhelper.MCPEndpoint, strings.NewReader(`{}`)))

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Header().Get("WWW-Authenticate"),
		`resource_metadata="`+splitMCPURL+`/.well-known/oauth-protected-resource/mcp"`)
}
