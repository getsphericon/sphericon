package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An oversized JSON body is cut off before the ogen decoder buffers it, on the
// public collect surface (own, smaller caps) and on the authenticated API.
func TestOversizedBodiesAreRejectedThroughTheRealServer(t *testing.T) {
	env := testhelper.Setup(t)
	pad := func(n int) string { return `{"pad":"` + strings.Repeat("a", n) + `"}` }
	post := func(path, body string, headers map[string]string) *httptest.ResponseRecorder {
		return servePost(t, env, path, body, headers)
	}

	collect := post("/collect/events", pad(600<<10), map[string]string{"x-collect-key": fixtures.AcmeCollectKey})
	assert.Equal(t, http.StatusRequestEntityTooLarge, collect.Code)
	assert.Equal(t, "application/problem+json", collect.Header().Get("Content-Type"))

	api := post("/api/contacts", pad(1<<20), map[string]string{"Authorization": "Bearer " + env.ScopedBearer(t)})
	assert.Equal(t, http.StatusRequestEntityTooLarge, api.Code)
	assert.Equal(t, "application/problem+json", api.Header().Get("Content-Type"))

	// Within the cap the body is read normally (here it fails validation, not size).
	small := post("/collect/events", pad(1<<10), map[string]string{"x-collect-key": fixtures.AcmeCollectKey})
	assert.NotEqual(t, http.StatusRequestEntityTooLarge, small.Code)
}

// servePost sends one JSON POST through the composed server.
func servePost(t *testing.T, env *testhelper.TestEnv, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	return rec
}

// bodyCap is a small default cap, so the boundary tests below are exact and cheap.
const bodyCap = 4 << 10

func withSmallBodyCap() testhelper.Option {
	return testhelper.WithConfig(func(c *config.Config) { c.BodyLimits.Default = bodyCap })
}

// jsonRPCPingOfSize is a valid JSON-RPC ping request that is exactly n bytes long.
func jsonRPCPingOfSize(t *testing.T, n int) string {
	t.Helper()
	const frame = `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"pad":""}}`
	require.GreaterOrEqual(t, n, len(frame))
	return strings.Replace(frame, `"pad":""`, `"pad":"`+strings.Repeat("a", n-len(frame))+`"`, 1)
}

// The SES hook relies on the global cap: an oversized notification is a 413
// problem and is refused while the body is read, before any signature check.
func TestSESHookOversizedBodyIsRejected(t *testing.T) {
	env := testhelper.Setup(t, withSmallBodyCap())
	rec := servePost(t, env, "/hooks/"+fixtures.AcmeIngestKey+"/ses", `{"Message":"`+strings.Repeat("a", bodyCap)+`"}`, nil)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
}

// /mcp is read by the MCP SDK, not an ogen decoder: an oversized body must still
// come back as a 413, while a body of exactly the cap is read normally.
func TestMCPOversizedBodyIsRejected(t *testing.T) {
	env := testhelper.Setup(t, withSmallBodyCap())
	post := func(body string) *httptest.ResponseRecorder {
		return servePost(t, env, testhelper.MCPEndpoint, body, map[string]string{
			"Accept":        "application/json, text/event-stream",
			"Authorization": "Bearer " + env.ScopedBearer(t),
		})
	}

	assert.Equal(t, http.StatusRequestEntityTooLarge, post(jsonRPCPingOfSize(t, bodyCap+1)).Code)
	assert.Equal(t, http.StatusOK, post(jsonRPCPingOfSize(t, bodyCap)).Code)
}
