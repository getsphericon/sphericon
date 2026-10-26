package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/go-chi/chi/v5"
)

// TestRecovererLogsPanicWithRequestID verifies the middleware chain order:
// recoverer wraps requestID (ADR 0025 order), yet a recovered panic is logged with the
// correlation id, and the client still gets a 500 (not a hung/blank response).
func TestRecovererLogsPanicWithRequestID(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	boom := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})
	handler := chi.Chain(recoverer, requestID).Handler(boom)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/explode", nil)
	req.Header.Set("X-Request-ID", "req-abc")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equalf(t, http.StatusInternalServerError, rec.Code, "status = %d, want 500", rec.Code)
	got := rec.Header().Get("X-Request-ID")
	require.Equalf(t, "req-abc", got, "X-Request-Id header = %q, want req-abc", got)

	var line map[string]any
	err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line)
	require.NoErrorf(t, err, "parse panic log line %q: %v", buf.String(), err)
	require.Equalf(t, "panic recovered", line["msg"], "msg = %v, want 'panic recovered'", line["msg"])
	require.Equalf(t, "req-abc", line["request_id"], "request_id = %v, want req-abc", line["request_id"])
	require.Equalf(t, "/explode", line["path"], "path = %v, want /explode", line["path"])
	stack, _ := line["stack"].(string)
	require.Contains(t, stack, "logging_middleware_test.go", "stack does not point at the panic site")
}

// captureLogs routes the default logger to a buffer at debug level for the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func serveAccessLogged(t *testing.T, path string, status int) {
	t.Helper()
	handler := chi.Chain(requestID, accessLog).Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path+"?token=secret", nil)
	req.Header.Set("X-Request-ID", "req-abc")
	handler.ServeHTTP(httptest.NewRecorder(), req)
}

func accessLogLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var line map[string]any
	require.NoErrorf(t, json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line), "parse log line %q", buf.String())
	return line
}

func TestAccessLogWritesOneLinePerRequest(t *testing.T) {
	buf := captureLogs(t)

	serveAccessLogged(t, "/site/workspaces/1/broadcasts", http.StatusUnprocessableEntity)

	line := accessLogLine(t, buf)
	require.Equal(t, "request", line["msg"])
	require.Equal(t, "WARN", line["level"])
	require.Equal(t, "req-abc", line["request_id"])
	require.Equal(t, "POST", line["method"])
	require.Equal(t, "/site/workspaces/1/broadcasts", line["path"])
	require.InDelta(t, http.StatusUnprocessableEntity, line["status"], 0)
	require.NotContains(t, buf.String(), "secret", "the query string is not logged")
}

func TestAccessLogRedactsACredentialInThePath(t *testing.T) {
	buf := captureLogs(t)

	serveAccessLogged(t, "/e/c/signed-token-value", http.StatusFound)

	require.Contains(t, buf.String(), "/e/c/{redacted}")
	require.NotContains(t, buf.String(), "signed-token-value")
}

func TestAccessLogSkipsProbes(t *testing.T) {
	buf := captureLogs(t)

	serveAccessLogged(t, "/healthz", http.StatusOK)
	serveAccessLogged(t, "/readyz", http.StatusOK)

	require.Empty(t, buf.String())
}

func TestAccessLogLogsAServerFaultAsAnError(t *testing.T) {
	buf := captureLogs(t)

	serveAccessLogged(t, "/api/contacts", http.StatusInternalServerError)

	require.Equal(t, "ERROR", accessLogLine(t, buf)["level"])
}

func TestAccessLogLogsASuccessOnThePixelAsDebug(t *testing.T) {
	buf := captureLogs(t)

	serveAccessLogged(t, "/e/o/pixel", http.StatusOK)

	require.Equal(t, "DEBUG", accessLogLine(t, buf)["level"])
}

func TestAccessLogLogsASuccessAsInfo(t *testing.T) {
	buf := captureLogs(t)

	serveAccessLogged(t, "/site/session", http.StatusOK)

	require.Equal(t, "INFO", accessLogLine(t, buf)["level"])
}
