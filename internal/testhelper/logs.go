package testhelper

import (
	"bytes"
	"log/slog"
	"sync"
	"testing"
)

// Logs is what the default logger wrote while a test captured it (CaptureLogs), as slog's text
// handler writes it. Code under test may log from a goroutine of its own while the test reads.
type Logs struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *Logs) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

// String is everything logged so far.
func (l *Logs) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// CaptureLogs points the default logger at the returned Logs until the test ends. The logger is
// the process's, so a test that captures it does not run in parallel with one that logs.
func CaptureLogs(t *testing.T) *Logs {
	t.Helper()
	logs := &Logs{}
	capture(t, slog.NewTextHandler(logs, nil))
	return logs
}

// CaptureJSONLogs is CaptureLogs written by slog's JSON handler, the production format, for a test
// that reads a record's level or attributes as they are shipped.
func CaptureJSONLogs(t *testing.T) *Logs {
	t.Helper()
	logs := &Logs{}
	capture(t, slog.NewJSONHandler(logs, nil))
	return logs
}

func capture(t *testing.T, h slog.Handler) {
	t.Helper()
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })
}
