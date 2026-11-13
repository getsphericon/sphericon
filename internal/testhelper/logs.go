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
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return logs
}
