package server_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/testhelper"
)

// captureLogs routes the default logger to a buffer at debug level for the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// logLines are the JSON lines written to buf.
func logLines(t require.TestingT, buf *bytes.Buffer) []map[string]any {
	var lines []map[string]any
	dec := json.NewDecoder(buf)
	for {
		var line map[string]any
		err := dec.Decode(&line)
		if errors.Is(err, io.EOF) {
			return lines
		}
		require.NoError(t, err, "parse log line")
		lines = append(lines, line)
	}
}

// linesWithMsg are the lines of buf whose message is msg.
func linesWithMsg(t require.TestingT, buf *bytes.Buffer, msg string) []map[string]any {
	return lo.Filter(logLines(t, buf), func(line map[string]any, _ int) bool { return line["msg"] == msg })
}

// onlyLine is the one line of buf whose message is msg.
func onlyLine(t require.TestingT, buf *bytes.Buffer, msg string) map[string]any {
	lines := linesWithMsg(t, buf, msg)
	require.Len(t, lines, 1, "log lines %q", msg)
	return lines[0]
}

// A recovered panic answers a 500 problem and is logged with the correlation id, although the
// recoverer sits outside the request id (ADR 0025 order).
func TestAPanicIsAnswered500AndLoggedWithTheRequestID(t *testing.T) {
	buf := captureLogs(t)
	h := internalServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))

	rec := serveRequest(t, h, http.MethodGet, "/service/explode", map[string]string{"X-Request-ID": "req-abc"})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	assert.Equal(t, "req-abc", rec.Header().Get("X-Request-ID"))
	line := onlyLine(t, buf, "panic recovered")
	assert.Equal(t, "req-abc", line["request_id"])
	assert.Equal(t, "/service/explode", line["path"])
	assert.Contains(t, line["stack"], "logging_middleware_test.go", "the stack points at the panic site")
}

func TestAccessLogWritesOneLinePerRequest(t *testing.T) {
	buf := captureLogs(t)
	h := internalServer(t, statusHandler(http.StatusUnprocessableEntity))

	serveRequest(t, h, http.MethodPost, "/service/workspaces/1?token=secret", map[string]string{"X-Request-ID": "req-abc"})

	line := onlyLine(t, buf, "request")
	assert.Equal(t, "WARN", line["level"])
	assert.Equal(t, "req-abc", line["request_id"])
	assert.Equal(t, "POST", line["method"])
	assert.Equal(t, "/service/workspaces/1", line["path"])
	assert.InDelta(t, http.StatusUnprocessableEntity, line["status"], 0)
	assert.NotContains(t, buf.String(), "secret", "the query string is not logged")
}

func TestAccessLogRedactsACredentialInThePath(t *testing.T) {
	buf := captureLogs(t)
	h := internalServer(t, statusHandler(http.StatusOK))

	serveRequest(t, h, http.MethodGet, "/e/c/signed-token-value", nil)

	assert.Contains(t, buf.String(), "/e/c/{redacted}")
	assert.NotContains(t, buf.String(), "signed-token-value")
}

func TestAccessLogSkipsProbes(t *testing.T) {
	env := testhelper.Setup(t)
	buf := captureLogs(t)

	serveRequest(t, env.Server, http.MethodGet, "/healthz", nil)
	serveRequest(t, env.Server, http.MethodGet, "/readyz", nil)

	assert.Empty(t, linesWithMsg(t, buf, "request"))
}

func TestAccessLogLogsAServerFaultAsAnError(t *testing.T) {
	buf := captureLogs(t)
	h := internalServer(t, statusHandler(http.StatusInternalServerError))

	serveRequest(t, h, http.MethodGet, "/service/contacts", nil)

	assert.Equal(t, "ERROR", onlyLine(t, buf, "request")["level"])
}

// The open pixel answers every hit, a bad token included, and a page or a mailbox calls it
// constantly: its success is debug.
func TestAccessLogLogsASuccessOnThePixelAsDebug(t *testing.T) {
	env := testhelper.Setup(t)
	buf := captureLogs(t)

	rec := serveRequest(t, env.Server, http.MethodGet, "/e/o/not-a-token", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "DEBUG", onlyLine(t, buf, "request")["level"])
}

func TestAccessLogLogsASuccessAsInfo(t *testing.T) {
	buf := captureLogs(t)
	h := internalServer(t, statusHandler(http.StatusOK))

	serveRequest(t, h, http.MethodGet, "/service/session", nil)

	assert.Equal(t, "INFO", onlyLine(t, buf, "request")["level"])
}
