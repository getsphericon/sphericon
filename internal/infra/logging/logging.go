// Package logging builds the application's structured logger (log/slog) and
// carries the request id and the correlation id through context.
//
// Wiring model: the process installs one configured logger as slog.Default via
// Setup, so every subsystem that already logs through slog (river and the HTTP
// handlers) shares the same level, format,
// and sink without threading a *slog.Logger through every constructor. Request
// scoping is done via context: a line logged with slog's *Context functions is
// tagged with the ids its context carries (WithRequestID, WithCorrelationID).
package logging

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/getsphericon/sphericon/config"
	"github.com/lmittmann/tint"
	"github.com/riverqueue/river/riverlog"
	slogctx "github.com/veqryn/slog-context"
)

// New builds a logger from config, writing to w: a coloured, human-readable handler for
// LogFormat "text" (dev), otherwise structured JSON (prod), from the configured level up. Every
// line logged with a context is tagged with the request id and the correlation id it carries,
// and a line logged with a running job's context is also kept on the job.
func New(cfg *config.Config, w io.Writer) *slog.Logger {
	var handler slog.Handler
	if cfg.LogFormat == config.LogFormatText {
		handler = tint.NewTextHandler(w, &tint.Options{Level: cfg.LogLevel})
	} else {
		handler = slog.NewJSONHandler(w, &slog.HandlerOptions{Level: cfg.LogLevel})
	}
	return slog.New(slogctx.NewHandler(jobLogs{handler}, &slogctx.HandlerOptions{
		Prependers: []slogctx.AttrExtractor{contextIDs},
		Appenders:  []slogctx.AttrExtractor{}, // none: the ids are the only attributes a context adds
	}))
}

// contextIDs are the attributes of the ids ctx carries: the request id and the correlation id.
func contextIDs(ctx context.Context, _ time.Time, _ slog.Level, _ string) []slog.Attr {
	var attrs []slog.Attr
	if id, ok := RequestIDFromContext(ctx); ok && id != "" {
		attrs = append(attrs, slog.String("request_id", id))
	}
	if id, ok := correlationIDFromContext(ctx); ok && id != "" {
		attrs = append(attrs, slog.String("correlation_id", id))
	}
	return attrs
}

type requestIDKey struct{}

// WithRequestID returns a context carrying the id of the HTTP request the work in it serves, so
// the lines logged with it are tagged with the id.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFromContext returns the request id carried by ctx, if any.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(requestIDKey{}).(string)
	return id, ok
}

type correlationKey struct{}

// WithCorrelationID returns a context whose logs are tagged with the correlation id of the chain
// of domain events the work in it belongs to.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationKey{}, id)
}

func correlationIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(correlationKey{}).(string)
	return id, ok
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
// that job's log, then hands it on. The copy carries every attribute of the line, the context's
// ids and those a logger added with With included: New puts jobLogs under slogctx, which folds
// them all into the record.
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
// river and every slog.Default() call site emit through it.
func Setup(cfg *config.Config) {
	slog.SetDefault(New(cfg, os.Stderr))
}
