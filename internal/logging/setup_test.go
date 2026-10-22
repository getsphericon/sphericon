package logging

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
)

func TestSetupInstallsDefaultLogger(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	Setup(&config.Config{LogFormat: "json", LogLevel: "error"})

	def := slog.Default()
	warnEnabled := def.Enabled(context.Background(), slog.LevelWarn)
	errorEnabled := def.Enabled(context.Background(), slog.LevelError)
	require.False(t, warnEnabled, "default logger should be at error level after Setup")
	require.True(t, errorEnabled, "default logger should emit errors after Setup")
}

func TestNewTextFormatIsCaseInsensitive(t *testing.T) {
	logger := New(&config.Config{LogFormat: "TEXT", LogLevel: "debug"})
	debugEnabled := logger.Enabled(context.Background(), slog.LevelDebug)
	require.True(t, debugEnabled, "text handler at debug level should emit debug")
}

func TestParseLevelNamedLevels(t *testing.T) {
	for _, c := range []struct {
		in   string
		want slog.Level
	}{
		{"warn", slog.LevelWarn},
		{" Warning ", slog.LevelWarn},
		{"error", slog.LevelError},
		{"info", slog.LevelInfo},
		{"", slog.LevelInfo},
	} {
		got := parseLevel(c.in)
		assert.Equalf(t, c.want, got, "parseLevel(%q) = %v, want %v", c.in, got, c.want)
	}
}
