package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/telemetry"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
)

func TestHealthz(t *testing.T) {
	env := testhelper.Setup(t)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	env.Server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"status":"ok"}`, w.Body.String())
}

func TestReadyz(t *testing.T) {
	env := testhelper.Setup(t)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	env.Server.ServeHTTP(w, req)

	// The txdb test connection is live, so readiness reports OK.
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"status":"ok"}`, w.Body.String())
}

// Metrics live on the opt-in internal listener (ADR 0018), never on the public
// port. Telemetry is set up so that a re-mounted route would serve the exposition.
// The catch-all answers unknown paths with the SPA shell or the not-embedded hint,
// so /metrics must answer exactly like any other unknown path.
func TestPublicHandlerDoesNotServeMetrics(t *testing.T) {
	testhelper.InstallOtel(t, func(ctx context.Context) (func(context.Context) error, error) {
		return telemetry.Setup(ctx, &config.Config{OtelServiceName: "sphericon-test"}, "test", telemetry.BuildInfo{})
	})
	env := testhelper.Setup(t)

	serve := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		env.Server.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		return w
	}

	assert.Equal(t, http.StatusOK, serve("/healthz").Code, "the server is up and routing")

	metrics, unknown := serve("/metrics"), serve("/definitely-unknown")
	assert.Equal(t, unknown.Code, metrics.Code)
	assert.Equal(t, unknown.Body.String(), metrics.Body.String())
	assert.NotContains(t, metrics.Body.String(), "go_goroutine_count")
	assert.NotContains(t, metrics.Body.String(), "target_info")
}
