package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/logging"
)

func TestFromContextTagsRequestID(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ctx := logging.WithRequestID(context.Background(), "req-123")
	logging.FromContext(ctx).Info("hello")

	var rec map[string]any
	err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec)
	require.NoErrorf(t, err, "parse log line: %v", err)
	require.Equalf(t, "req-123", rec["request_id"], "request_id = %v, want req-123", rec["request_id"])
}

func TestFromContextWithoutRequestID(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	logging.FromContext(context.Background()).Info("hello")
	require.NotContainsf(t, buf.String(), "request_id", "did not expect request_id attr: %s", buf.String())
}
