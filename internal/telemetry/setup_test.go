package telemetry_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/telemetry"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func setup(build telemetry.BuildInfo) func(context.Context) (func(context.Context) error, error) {
	return func(ctx context.Context) (func(context.Context) error, error) {
		return telemetry.Setup(ctx, &config.Config{OtelServiceName: "sphericon-test"}, "test", build)
	}
}

func scrape(t *testing.T) (status int, body string) {
	t.Helper()
	rec := httptest.NewRecorder()
	telemetry.MetricsHandler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil))
	raw, err := io.ReadAll(rec.Result().Body)
	require.NoError(t, err)
	return rec.Code, string(raw)
}

// Once telemetry is shut down, /metrics answers 503 again instead of a stale exposition.
func TestMetricsHandlerIsA503StubAfterShutdown(t *testing.T) {
	testhelper.IsolateOtel(t)
	stop, err := setup(telemetry.BuildInfo{})(context.Background())
	require.NoError(t, err)
	require.NoError(t, stop(context.Background()))

	code, body := scrape(t)
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Contains(t, body, "telemetry not configured")
}

// Without a config file Setup serves Prometheus metrics, tags the resource
// and leaves the tracer provider a no-op.
func TestSetupPullOnly(t *testing.T) {
	testhelper.InstallOtel(t, setup(telemetry.BuildInfo{Version: "v1", Commit: "abc"}))

	code, body := scrape(t)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, "go_goroutine_count", "runtime metrics are exported on /metrics")
	assert.Contains(t, body, "target_info")
	assert.Contains(t, body, `service_name="sphericon-test"`)
	assert.Contains(t, body, `service_version="v1"`)
	assert.Contains(t, body, `service_commit="abc"`)
	assert.Regexp(t, `sphericon_build_info\{[^}]*commit="abc"[^}]*\} 1`, body)
	assert.Regexp(t, `sphericon_build_info\{[^}]*version="v1"[^}]*\} 1`, body)

	_, span := otel.Tracer("test").Start(context.Background(), "work")
	defer span.End()
	assert.False(t, span.SpanContext().IsValid(), "the tracer provider is a no-op without a config file")
	assert.Contains(t, otel.GetTextMapPropagator().Fields(), "traceparent")
}

// sphericon_license_expired is 1 while the license key has lapsed and 0 otherwise (ADR 0035).
func TestLicenseExpiredGaugeFollowsTheLapse(t *testing.T) {
	testhelper.InstallOtel(t, setup(telemetry.BuildInfo{Version: "0.1.0", Commit: "abc"}))
	var lapsed atomic.Bool
	require.NoError(t, telemetry.ObserveLicenseExpired(lapsed.Load))

	_, body := scrape(t)
	assert.Regexp(t, `sphericon_license_expired(\{[^}]*\})? 0`, body)

	lapsed.Store(true)
	_, body = scrape(t)
	assert.Regexp(t, `sphericon_license_expired(\{[^}]*\})? 1`, body)
}

// The newer release the Release check cached shows next to sphericon_build_info, and the
// gauge disappears once there is none.
func TestReleaseAvailableGaugeFollowsTheCachedResult(t *testing.T) {
	testhelper.InstallOtel(t, setup(telemetry.BuildInfo{Version: "0.1.0", Commit: "abc"}))
	var latest atomic.Value
	latest.Store("0.2.0")
	require.NoError(t, telemetry.ObserveReleaseAvailable(func(context.Context) (string, error) { return latest.Load().(string), nil }))

	_, body := scrape(t)
	assert.Regexp(t, `sphericon_release_available\{[^}]*latest="0.2.0"[^}]*\} 1`, body)
	assert.Contains(t, body, "sphericon_build_info")

	latest.Store("")
	_, body = scrape(t)
	assert.NotContains(t, body, "sphericon_release_available{")
}

// With OTEL_CONFIG_FILE naming an OTLP exporter, traces and metrics push to it and shutdown
// flushes both.
func TestSetupWithOTLPPush(t *testing.T) {
	testhelper.IsolateOtel(t)

	var hits atomic.Int64
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(collector.Close)
	file := filepath.Join(t.TempDir(), "otel.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`file_format: "1.0"
tracer_provider:
  processors:
    - simple:
        exporter:
          otlp_http:
            endpoint: `+collector.URL+`/v1/traces
meter_provider:
  readers:
    - periodic:
        interval: 100000
        exporter:
          otlp_http:
            endpoint: `+collector.URL+`/v1/metrics
`), 0o600))
	t.Setenv("OTEL_CONFIG_FILE", file)

	ctx := context.Background()
	stop, err := setup(telemetry.BuildInfo{})(ctx)
	require.NoError(t, err)

	_, span := otel.Tracer("test").Start(ctx, "work")
	assert.True(t, span.SpanContext().IsValid(), "a real tracer provider is installed by the config file")
	span.End()

	require.NoError(t, stop(ctx))
	assert.Positive(t, hits.Load(), "shutdown flushed spans/metrics to the collector")
}
