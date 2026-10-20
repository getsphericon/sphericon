package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
)

func TestFromContextTagsRequestID(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ctx := WithRequestID(context.Background(), "req-123")
	FromContext(ctx).Info("hello")

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

	FromContext(context.Background()).Info("hello")
	require.NotContainsf(t, buf.String(), "request_id", "did not expect request_id attr: %s", buf.String())
}

func TestNewHonoursFormatAndLevel(t *testing.T) {
	// JSON handler at warn level: an info line is dropped, a warn line kept.
	logger := New(&config.Config{LogFormat: "json", LogLevel: "warn"})
	infoEnabled := logger.Enabled(context.Background(), slog.LevelInfo)
	warnEnabled := logger.Enabled(context.Background(), slog.LevelWarn)
	require.False(t, infoEnabled, "info should be disabled at warn level")
	require.True(t, warnEnabled, "warn should be enabled at warn level")
}

func TestParseLevelDefaultsToInfo(t *testing.T) {
	got := parseLevel("nonsense")
	require.Equalf(t, slog.LevelInfo, got, "parseLevel(nonsense) = %v, want info", got)
	got = parseLevel("DEBUG")
	require.Equalf(t, slog.LevelDebug, got, "parseLevel(DEBUG) = %v, want debug", got)
}
