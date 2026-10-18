// Package telemetry wires OpenTelemetry traces, metrics and logs for the process.
//
// The SDK is built by otelconf from OpenTelemetry's declarative configuration:
// the staff member points OTEL_CONFIG_FILE at a YAML file and decides there which
// signals go to which destination (any exporter, several at once, sampling,
// views). Without the file the process exports nothing over OTLP, so there is
// zero overhead out of the box (and in the test suite).
//
// Wiring model mirrors the logging package: Setup installs the global OTel
// providers (otel.SetTracerProvider / otel.SetMeterProvider / propagator) once
// at boot, so every already-instrumented subsystem picks them up without being
// threaded a provider. In particular the ogen-generated HTTP servers fall back
// to otel.GetTracerProvider()/GetMeterProvider() (see gen/*/oas_cfg_gen.go), so
// all three API surfaces (site/external/collect) get request spans + HTTP
// metrics from the globals alone.
//
// Metrics are also exposed as a Prometheus /metrics endpoint (pull), served only
// by the opt-in listener on SPHERICON_METRICS_ADDR (see MetricsServer, ADR 0018), never on
// the public port. That reader is attached to the same MeterProvider whatever the
// file says, so a file without a meter_provider section still serves /metrics.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/getsphericon/sphericon/config"
	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/contrib/otelconf"
	"go.opentelemetry.io/otel"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// configFileEnv names the declarative configuration file, per the OpenTelemetry
// SDK environment variable specification.
const configFileEnv = "OTEL_CONFIG_FILE"

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
	otelCfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	addResource(otelCfg, cfg, env, build)
	if otelCfg.MeterProvider == nil {
		otelCfg.MeterProvider = &otelconf.MeterProvider{}
	}
	// Inbound W3C trace context must be stitched (ogen reads the global propagator)
	// even when the file names none.
	if otelCfg.Propagator == nil {
		otelCfg.Propagator = &otelconf.Propagator{Composite: []otelconf.TextMapPropagator{
			{Tracecontext: otelconf.TraceContextPropagator{}},
			{Baggage: otelconf.BaggagePropagator{}},
		}}
	}

	// Prometheus reader: always on. A dedicated registry keeps the exposition
	// independent of the global default registry.
	reg := promclient.NewRegistry()
	promReader, err := otelprom.New(otelprom.WithRegisterer(reg))
	if err != nil {
		return nil, err
	}

	sdk, err := newSDK(ctx, *otelCfg, sdkmetric.WithReader(promReader))
	if err != nil {
		return nil, fmt.Errorf("initialize OpenTelemetry SDK: %w", err)
	}
	otel.SetTracerProvider(sdk.TracerProvider())
	otel.SetMeterProvider(sdk.MeterProvider())
	otel.SetTextMapPropagator(sdk.Propagator())

	// Go runtime metrics (GC, goroutines, memory) into the same meter provider.
	if err := runtime.Start(runtime.WithMeterProvider(sdk.MeterProvider())); err != nil {
		return nil, errors.Join(err, sdk.Shutdown(ctx))
	}

	metricsHandler = promhttp.HandlerFor(reg, promhttp.HandlerOpts{})

	return func(ctx context.Context) error {
		metricsHandler = stubMetricsHandler
		return sdk.Shutdown(ctx)
	}, nil
}

// loadConfig parses the file OTEL_CONFIG_FILE names (environment references in it
// are resolved by otelconf), or returns an empty configuration when it is unset.
func loadConfig() (*otelconf.OpenTelemetryConfiguration, error) {
	path := os.Getenv(configFileEnv)
	if path == "" {
		return &otelconf.OpenTelemetryConfiguration{FileFormat: "1.0"}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", configFileEnv, err)
	}
	parsed, err := otelconf.ParseYAML(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", configFileEnv, err)
	}
	return parsed, nil
}

// addResource identifies the build on every signal. Attributes the file already
// sets are the staff member's and stay as they are.
func addResource(otelCfg *otelconf.OpenTelemetryConfiguration, cfg *config.Config, env string, build BuildInfo) {
	if otelCfg.Resource == nil {
		otelCfg.Resource = &otelconf.Resource{}
	}
	have := map[string]bool{}
	for _, a := range otelCfg.Resource.Attributes {
		have[a.Name] = true
	}
	for _, a := range []struct{ name, value string }{
		{"service.name", cfg.OtelServiceName},
		{"service.version", build.Version},
		{"service.commit", build.Commit},
		{"deployment.environment", env},
	} {
		if !have[a.name] {
			otelCfg.Resource.Attributes = append(otelCfg.Resource.Attributes, otelconf.AttributeNameValue{Name: a.name, Value: a.value})
		}
	}
}

// newSDK builds the SDK from the configuration given here. otelconf would re-read
// OTEL_CONFIG_FILE and let it supersede this configuration (dropping the resource
// and the Prometheus reader added above), so the variable is hidden for the call.
func newSDK(ctx context.Context, otelCfg otelconf.OpenTelemetryConfiguration, meterOpts ...sdkmetric.Option) (otelconf.SDK, error) {
	if path, ok := os.LookupEnv(configFileEnv); ok {
		_ = os.Unsetenv(configFileEnv)
		defer func() { _ = os.Setenv(configFileEnv, path) }()
	}
	return otelconf.NewSDK(
		otelconf.WithContext(ctx),
		otelconf.WithOpenTelemetryConfiguration(otelCfg),
		otelconf.WithMeterProviderOptions(meterOpts...),
	)
}
