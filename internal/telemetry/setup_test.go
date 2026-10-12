package telemetry

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var otlpEnvKeys = []string{
	"OTEL_EXPORTER_OTLP_ENDPOINT",
	"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
	"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
}

// isolate restores the process-wide state Setup mutates and clears the OTLP env.
func isolate(t *testing.T) {
	t.Helper()
	testhelper.IsolateOtel(t)
	prevHandler := metricsHandler
	t.Cleanup(func() { metricsHandler = prevHandler })
}

func scrape(t *testing.T) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	MetricsHandler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil))
	body, err := io.ReadAll(rec.Result().Body)
	require.NoError(t, err)
	return rec.Code, string(body)
}

func TestMetricsHandlerIsA503StubBeforeSetup(t *testing.T) {
	isolate(t)
	metricsHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "telemetry not configured", http.StatusServiceUnavailable)
	})

	code, body := scrape(t)
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Contains(t, body, "telemetry not configured")
}

func TestOTLPEnabled(t *testing.T) {
	isolate(t)
	assert.False(t, otlpEnabled())

	for _, k := range otlpEnvKeys {
		t.Run(k, func(t *testing.T) {
			t.Setenv(k, "http://collector:4318")
			assert.True(t, otlpEnabled())
		})
	}
}

// Without an OTLP endpoint Setup serves Prometheus metrics, tags the resource
// and leaves the tracer provider alone.
func TestSetupPullOnly(t *testing.T) {
	isolate(t)
	tpBefore := otel.GetTracerProvider()

	stop, err := Setup(context.Background(), &config.Config{OtelServiceName: "sphericon-test"}, "test", BuildInfo{Version: "v1", Commit: "abc"})
	require.NoError(t, err)

	code, body := scrape(t)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, "go_goroutine_count", "runtime metrics are exported on /metrics")
	assert.Contains(t, body, "target_info")
	assert.Contains(t, body, `service_name="sphericon-test"`)
	assert.Contains(t, body, `service_version="v1"`)
	assert.Contains(t, body, `service_commit="abc"`)

	assert.Same(t, tpBefore, otel.GetTracerProvider(), "no tracer provider is registered without OTLP")
	assert.Contains(t, otel.GetTextMapPropagator().Fields(), "traceparent")

	require.NoError(t, stop(context.Background()))
}

// With an OTLP endpoint configured, traces and metrics push to it and shutdown
// flushes both.
func TestSetupWithOTLPPush(t *testing.T) {
	isolate(t)

	var hits atomic.Int64
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(collector.Close)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)

	ctx := context.Background()
	stop, err := Setup(ctx, &config.Config{OtelServiceName: "sphericon-test"}, "test", BuildInfo{})
	require.NoError(t, err)

	_, span := otel.Tracer("test").Start(ctx, "work")
	assert.True(t, span.SpanContext().IsValid(), "a real tracer provider is installed when OTLP is on")
	span.End()

	require.NoError(t, stop(ctx))
	assert.Positive(t, hits.Load(), "shutdown flushed spans/metrics to the collector")
}
