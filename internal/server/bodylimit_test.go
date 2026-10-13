package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
)

// An oversized JSON body is cut off before the ogen decoder buffers it, on the
// public collect surface (own, smaller caps) and on the authenticated API.
func TestOversizedBodiesAreRejectedThroughTheRealServer(t *testing.T) {
	env := testhelper.Setup(t)
	pad := func(n int) string { return `{"pad":"` + strings.Repeat("a", n) + `"}` }
	post := func(path, body string, headers map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		env.Server.ServeHTTP(rec, req)
		return rec
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

// The SES hook relies on the global cap: an oversized notification is a 413 and
// never reaches the signature check.
func TestSESHookOversizedBodyIsRejected(t *testing.T) {
	env := testhelper.Setup(t)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/hooks/"+fixtures.AcmeIngestKey+"/ses",
		strings.NewReader(`{"Message":"`+strings.Repeat("a", 2<<20)+`"}`))
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

// /mcp is read by the MCP SDK, not an ogen decoder: an oversized body must still
// come back as a 413 (the SDK keeps the *http.MaxBytesError of the global cap).
func TestMCPOversizedBodyIsRejected(t *testing.T) {
	env := testhelper.Setup(t)
	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, testhelper.MCPEndpoint, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+env.ScopedBearer(t))
		rec := httptest.NewRecorder()
		env.Server.ServeHTTP(rec, req)
		return rec
	}
	pad := func(n int) string { return `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"pad":"` + strings.Repeat("a", n) + `"}}` }

	assert.Equal(t, http.StatusRequestEntityTooLarge, post(pad(1<<20)).Code)

	// Within the cap the body is read normally.
	assert.NotEqual(t, http.StatusRequestEntityTooLarge, post(pad(1<<10)).Code)
}
