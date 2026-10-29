package logging_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/logging"
)

func TestSetupInstallsDefaultLogger(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	logging.Setup(&config.Config{LogFormat: "json", LogLevel: "error"})

	def := slog.Default()
	warnEnabled := def.Enabled(context.Background(), slog.LevelWarn)
	errorEnabled := def.Enabled(context.Background(), slog.LevelError)
	require.False(t, warnEnabled, "default logger should be at error level after Setup")
	require.True(t, errorEnabled, "default logger should emit errors after Setup")
}

// The level is one of debug|info|warn|error, in any case and with surrounding spaces, and
// anything unrecognised is info: the logger emits that level and nothing below it.
func TestNewLogsFromTheConfiguredLevel(t *testing.T) {
	for _, c := range []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{" Warning ", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},
		{"nonsense", slog.LevelInfo},
	} {
		logger := logging.New(&config.Config{LogFormat: "json", LogLevel: c.in})

		assert.Truef(t, logger.Enabled(context.Background(), c.want), "LogLevel %q emits %v", c.in, c.want)
		assert.Falsef(t, logger.Enabled(context.Background(), c.want-1), "LogLevel %q drops what is below %v", c.in, c.want)
	}
}
