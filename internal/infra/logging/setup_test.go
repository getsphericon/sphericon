package logging_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/infra/logging"
)

func TestSetupInstallsDefaultLogger(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	logging.Setup(&config.Config{LogFormat: "json", LogLevel: slog.LevelError})

	def := slog.Default()
	warnEnabled := def.Enabled(context.Background(), slog.LevelWarn)
	errorEnabled := def.Enabled(context.Background(), slog.LevelError)
	require.False(t, warnEnabled, "default logger should be at error level after Setup")
	require.True(t, errorEnabled, "default logger should emit errors after Setup")
}

// The logger emits the configured level and nothing below it, in either format.
func TestNewLogsFromTheConfiguredLevel(t *testing.T) {
	for _, format := range []string{"json", "text"} {
		for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
			logger := logging.New(&config.Config{LogFormat: format, LogLevel: level}, io.Discard)

			assert.Truef(t, logger.Enabled(context.Background(), level), "%s at %v emits %v", format, level, level)
			assert.Falsef(t, logger.Enabled(context.Background(), level-1), "%s at %v drops what is below it", format, level)
		}
	}
}
