package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

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
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("parse log line: %v", err)
	}
	if rec["request_id"] != "req-123" {
		t.Fatalf("request_id = %v, want req-123", rec["request_id"])
	}
}

func TestFromContextWithoutRequestID(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	FromContext(context.Background()).Info("hello")
	if strings.Contains(buf.String(), "request_id") {
		t.Fatalf("did not expect request_id attr: %s", buf.String())
	}
}

func TestNewHonoursFormatAndLevel(t *testing.T) {
	// JSON handler at warn level: an info line is dropped, a warn line kept.
	logger := New(&config.Config{LogFormat: "json", LogLevel: "warn"})
	if logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("info should be disabled at warn level")
	}
	if !logger.Enabled(context.Background(), slog.LevelWarn) {
		t.Fatal("warn should be enabled at warn level")
	}
}

func TestParseLevelDefaultsToInfo(t *testing.T) {
	if got := parseLevel("nonsense"); got != slog.LevelInfo {
		t.Fatalf("parseLevel(nonsense) = %v, want info", got)
	}
	if got := parseLevel("DEBUG"); got != slog.LevelDebug {
		t.Fatalf("parseLevel(DEBUG) = %v, want debug", got)
	}
}
