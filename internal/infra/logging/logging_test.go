package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/infra/logging"
)

// installed makes the logger New builds the process's default, writing JSON to the returned
// buffer, for the length of the test.
func installed(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(logging.New(&config.Config{LogFormat: "json", LogLevel: slog.LevelInfo}, &buf))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func lines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range bytes.SplitSeq(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var rec map[string]any
		require.NoError(t, json.Unmarshal(line, &rec))
		out = append(out, rec)
	}
	return out
}

// A line logged through plain slog with a context carries the request id and the correlation id
// the context holds, also under a logger that added its own attributes and a group.
func TestALineLoggedWithAContextCarriesItsRequestAndCorrelationIDs(t *testing.T) {
	buf := installed(t)
	ctx := logging.WithCorrelationID(logging.WithRequestID(context.Background(), "01JREQUEST"), "01JCHAIN")

	slog.InfoContext(ctx, "handled")
	slog.Default().With("worker", "mailer").WithGroup("job").InfoContext(ctx, "sent", "attempt", 2)

	got := lines(t, buf)
	require.Len(t, got, 2)
	for _, rec := range got {
		require.Equal(t, "01JREQUEST", rec["request_id"])
		require.Equal(t, "01JCHAIN", rec["correlation_id"])
	}
	require.Equal(t, "mailer", got[1]["worker"])
	require.Equal(t, map[string]any{"attempt": float64(2)}, got[1]["job"])
}

// A line logged outside any request or chain of domain events carries neither id.
func TestALineLoggedWithoutIDsCarriesNone(t *testing.T) {
	buf := installed(t)

	slog.InfoContext(context.Background(), "alone")
	slog.InfoContext(logging.WithRequestID(context.Background(), ""), "empty")

	got := lines(t, buf)
	require.Len(t, got, 2)
	for _, rec := range got {
		require.NotContains(t, rec, "request_id")
		require.NotContains(t, rec, "correlation_id")
	}
}
