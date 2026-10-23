package testhelper

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
)

// IsolateOtel clears the OTel config file variable and restores the process-wide OTel globals when
// the test ends, so a test may call telemetry.Setup without leaking providers.
func IsolateOtel(t *testing.T) {
	t.Helper()
	prevTP := otel.GetTracerProvider()
	prevMP := otel.GetMeterProvider()
	prevProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetMeterProvider(prevMP)
		otel.SetTextMapPropagator(prevProp)
	})
	t.Setenv("OTEL_CONFIG_FILE", "")
}

// InstallOtel isolates the OTel globals (IsolateOtel), runs setup (telemetry.Setup
// bound to a config; passed in so telemetry's own tests can use it without an import
// cycle) and shuts it down when the test ends.
func InstallOtel(t *testing.T, setup func(context.Context) (func(context.Context) error, error)) {
	t.Helper()
	IsolateOtel(t)
	stop, err := setup(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = stop(context.WithoutCancel(t.Context())) })
}

// HTTPGet performs a GET against a real listener and returns the status and body.
func HTTPGet(t *testing.T, url string) (int, string) {
	t.Helper()
	code, body, err := TryHTTPGet(t.Context(), url)
	require.NoError(t, err)
	return code, body
}

// TryHTTPGet is HTTPGet without test assertions, safe to call from a polling
// goroutine such as require.Eventually.
func TryHTTPGet(ctx context.Context, url string) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return 0, "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", err
	}
	return resp.StatusCode, string(body), nil
}
