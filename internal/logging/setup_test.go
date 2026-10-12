package logging

import (
	"context"
	"log/slog"
	"testing"

	"github.com/getsphericon/sphericon/config"
)

func TestSetupInstallsDefaultLogger(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	Setup(&config.Config{LogFormat: "json", LogLevel: "error"})

	def := slog.Default()
	if def.Enabled(context.Background(), slog.LevelWarn) {
		t.Fatal("default logger should be at error level after Setup")
	}
	if !def.Enabled(context.Background(), slog.LevelError) {
		t.Fatal("default logger should emit errors after Setup")
	}
}

func TestNewTextFormatIsCaseInsensitive(t *testing.T) {
	logger := New(&config.Config{LogFormat: "TEXT", LogLevel: "debug"})
	if !logger.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("text handler at debug level should emit debug")
	}
}

func TestParseLevelNamedLevels(t *testing.T) {
	for in, want := range map[string]slog.Level{
		"warn":      slog.LevelWarn,
		" Warning ": slog.LevelWarn,
		"error":     slog.LevelError,
		"info":      slog.LevelInfo,
		"":          slog.LevelInfo,
	} {
		if got := parseLevel(in); got != want {
			t.Errorf("parseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}
