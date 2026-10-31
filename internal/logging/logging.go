// Package logging builds the application's structured logger (log/slog) and
// carries a per-request correlation id through context.
//
// Wiring model: the process installs one configured logger as slog.Default via
// Setup, so every subsystem that already logs through slog — river, watermill
// (via NewSlogLogger), and the HTTP handlers — shares the same level, format,
// and sink without threading a *slog.Logger through every constructor. Request
// scoping is done via context (FromContext), not by injecting a logger.
package logging

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/getsphericon/sphericon/config"
	"github.com/lmittmann/tint"
	"github.com/riverqueue/river/riverlog"
)

// New builds a logger from config: a coloured, human-readable handler for
// LogFormat "text" (dev), otherwise structured JSON (prod). Level is one of
// debug|info|warn|error, defaulting to info for anything unrecognised.
func New(cfg *config.Config) *slog.Logger {
	level := parseLevel(cfg.LogLevel)
	var handler slog.Handler
	if strings.EqualFold(cfg.LogFormat, "text") {
		handler = tint.NewTextHandler(os.Stderr, &tint.Options{Level: level})
	} else {
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	}
	return slog.New(jobLogs{handler})
}

// JobLogs is the river middleware that keeps the lines a job logs on the job itself (riverlog),
// where the Staff console's job queue UI shows them on the job's page. Together with the
// handler New builds, every line logged with the job's context is kept, not only those logged
// through riverlog.Logger.
func JobLogs() *riverlog.Middleware {
	return riverlog.NewMiddleware(func(w io.Writer) slog.Handler {
		return slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})
	}, nil)
}

// jobLogs copies a record logged with a running job's context (one JobLogs put a logger in) to
// that job's log, then hands it on. The copy carries the record's own attributes, not those a
// logger added with With.
type jobLogs struct{ slog.Handler }

func (h jobLogs) Handle(ctx context.Context, r slog.Record) error {
	var copied error
	if job, ok := riverlog.LoggerSafely(ctx); ok && job.Handler().Enabled(ctx, r.Level) {
		copied = job.Handler().Handle(ctx, r.Clone())
	}
	return errors.Join(h.Handler.Handle(ctx, r), copied)
}

func (h jobLogs) WithAttrs(attrs []slog.Attr) slog.Handler {
	return jobLogs{h.Handler.WithAttrs(attrs)}
}

func (h jobLogs) WithGroup(name string) slog.Handler { return jobLogs{h.Handler.WithGroup(name)} }

// Setup installs the configured logger as the process-wide slog default, so
// river, watermill, and every slog.Default() call site emit through it.
func Setup(cfg *config.Config) {
	slog.SetDefault(New(cfg))
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type ctxKey struct{}

// WithRequestID returns a context carrying the request correlation id, so logs
// emitted downstream via FromContext are tagged with it.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// RequestIDFromContext returns the request id carried by ctx, if any.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(ctxKey{}).(string)
	return id, ok
}

// FromContext returns the default logger, tagged with the request id when the
// context carries one. Use this at request-scoped log sites so every line
// correlates back to the originating HTTP request.
func FromContext(ctx context.Context) *slog.Logger {
	if id, ok := RequestIDFromContext(ctx); ok {
		return slog.Default().With("request_id", id)
	}
	return slog.Default()
}
