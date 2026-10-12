package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrackerScriptServed(t *testing.T) {
	env := testhelper.Setup(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/t.js", nil)
	env.Server.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, strings.HasPrefix(rec.Header().Get("Content-Type"), "application/javascript"),
		"Content-Type = %q", rec.Header().Get("Content-Type"))
	assert.NotEmpty(t, rec.Body.Bytes())
}

// Cross-origin ingestion: an arbitrary customer site must clear the CORS
// preflight against /collect, since the collect key is public.
func TestCollectCORSAllowsArbitraryOrigin(t *testing.T) {
	env := testhelper.Setup(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/collect/events", nil)
	req.Header.Set("Origin", "https://customer.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "x-collect-key,content-type")
	env.Server.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "https://customer.example", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers")), "x-collect-key")
	// Public key ⇒ no credentialed CORS (lets us echo any origin safely).
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Credentials"))
}
