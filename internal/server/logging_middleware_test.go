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
	req.Header.Set("X-Request-Id", "req-abc")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equalf(t, http.StatusInternalServerError, rec.Code, "status = %d, want 500", rec.Code)
	got := rec.Header().Get("X-Request-Id")
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
