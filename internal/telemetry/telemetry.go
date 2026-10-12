// Package telemetry wires OpenTelemetry traces and metrics for the process.
//
// Wiring model mirrors the logging package: Setup installs the global OTel
// providers (otel.SetTracerProvider / otel.SetMeterProvider / propagator) once
// at boot, so every already-instrumented subsystem picks them up without being
// threaded a provider. In particular the ogen-generated HTTP servers fall back
// to otel.GetTracerProvider()/GetMeterProvider() (see gen/*/oas_cfg_gen.go), so
// all three API surfaces (site/external/collect) get request spans + HTTP
// metrics from the globals alone.
//
// Metrics are exposed two ways off a single MeterProvider:
//   - a Prometheus /metrics endpoint (pull) — served only by the opt-in listener
//     on METRICS_ADDR (see MetricsServer, ADR 0018), never on the public port;
//   - an OTLP push exporter — enabled only when the standard OTEL_EXPORTER_OTLP_*
//     env vars point at a collector (dev grafana/otel-lgtm, or any OTLP backend).
//
// Traces have no pull model, so they export via OTLP only, gated on the same
// endpoint env. When no endpoint is set, no tracer provider is registered and
// ogen keeps its no-op global — zero overhead (e.g. in the test suite, which
// never calls Setup).
package telemetry

import (
	"context"
	"errors"
	"net/http"
	"os"

	"github.com/getsphericon/sphericon/config"
	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// BuildInfo carries the binary's version metadata into the OTel resource so
// traces/metrics are attributable to a specific build.
type BuildInfo struct {
	Version string
	Commit  string
}

// metricsHandler is the handler mounted at /metrics. It defaults to a 503 stub
// so server.New can mount it unconditionally even when Setup was never called
// (e.g. the test harness); Setup replaces it with the real Prometheus handler.
var metricsHandler http.Handler = stubMetricsHandler

var stubMetricsHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "telemetry not configured", http.StatusServiceUnavailable)
})

// MetricsHandler returns the Prometheus exposition handler for /metrics. Safe to
// call before Setup (returns a 503 stub until Setup runs).
func MetricsHandler() http.Handler { return metricsHandler }

// Setup installs the global OTel providers and returns a shutdown that flushes
// and stops them. Call it once at boot after logging.Setup; run the returned
// shutdown after the HTTP server has stopped so the final batched spans/metrics
// flush.
func Setup(ctx context.Context, cfg *config.Config, env string, build BuildInfo) (func(context.Context) error, error) {
	res := resource.NewSchemaless(
		attribute.String("service.name", cfg.OtelServiceName),
		attribute.String("service.version", build.Version),
		attribute.String("service.commit", build.Commit),
		attribute.String("deployment.environment", env),
	)

	// Prometheus reader: always on. A dedicated registry keeps the exposition
	// independent of the global default registry.
	reg := promclient.NewRegistry()
	promReader, err := otelprom.New(otelprom.WithRegisterer(reg))
	if err != nil {
		return nil, err
	}

	meterOpts := []sdkmetric.Option{
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(promReader),
	}

	push := otlpEnabled()

	// OTLP metric push reader (dev otel-lgtm / any OTLP backend), gated so it
	// never double-counts with the Prometheus scrape in prod.
	if push {
		metricExp, err := otlpmetrichttp.New(ctx)
		if err != nil {
			return nil, err
		}
		meterOpts = append(meterOpts, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)))
	}

	mp := sdkmetric.NewMeterProvider(meterOpts...)
	otel.SetMeterProvider(mp)

	// Traces: OTLP only (no pull model). Left as the no-op global when disabled.
	var tp *sdktrace.TracerProvider
	if push {
		spanExp, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, errors.Join(err, mp.Shutdown(ctx))
		}
		tp = sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(spanExp),
			sdktrace.WithResource(res),
		)
		otel.SetTracerProvider(tp)
	}

	// Propagator must be set for ogen to stitch inbound W3C trace context.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Go runtime metrics (GC, goroutines, memory) into the same meter provider.
	if err := runtime.Start(runtime.WithMeterProvider(mp)); err != nil {
		return nil, errors.Join(err, shutdown(ctx, tp, mp))
	}

	metricsHandler = promhttp.HandlerFor(reg, promhttp.HandlerOpts{})

	return func(ctx context.Context) error {
		metricsHandler = stubMetricsHandler
		return shutdown(ctx, tp, mp)
	}, nil
}

func shutdown(ctx context.Context, tp *sdktrace.TracerProvider, mp *sdkmetric.MeterProvider) error {
	var err error
	if tp != nil {
		err = errors.Join(err, tp.Shutdown(ctx))
	}
	return errors.Join(err, mp.Shutdown(ctx))
}

// otlpEnabled reports whether any standard OTLP endpoint env var is set, which
// gates the OTLP push exporters (traces + metrics).
func otlpEnabled() bool {
	for _, k := range []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
	} {
		if os.Getenv(k) != "" {
			return true
		}
	}
	return false
}
