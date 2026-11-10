package config

import "fmt"

// LogFormat is how a log line is written: JSON for a machine, or coloured text for a person.
type LogFormat string

const (
	// LogFormatJSON writes one JSON object per line, the default outside development.
	LogFormatJSON LogFormat = "json"
	// LogFormatText writes coloured, human-readable lines, the default in development.
	LogFormatText LogFormat = "text"
)

// UnmarshalText reads SPHERICON_LOG_FORMAT: anything but a known format is an error, so a typo
// never quietly means JSON.
func (f *LogFormat) UnmarshalText(text []byte) error {
	switch v := LogFormat(text); v {
	case LogFormatJSON, LogFormatText:
		*f = v
		return nil
	default:
		return fmt.Errorf("must be %q or %q, got %q", LogFormatJSON, LogFormatText, text)
	}
}
