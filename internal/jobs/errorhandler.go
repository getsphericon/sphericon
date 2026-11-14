package jobs

import (
	"context"
	"log/slog"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// errorHandler is river's centralized error/panic sink. Without it, a job that
// exhausts its retries is discarded silently and a worker panic only reaches
// river's default logger — both invisible in production. It logs each with
// enough context (kind, attempt, queue) to alert on, and is the natural seam for
// wiring Sentry later. It cancels nothing: a refusal is ended by RefusalsEnd before it gets
// here, and every other failure follows river's retry schedule.
type errorHandler struct {
	logger *slog.Logger
}

var _ river.ErrorHandler = (*errorHandler)(nil)

// NewErrorHandler builds river's error handler: the one the client runs with, exported so a
// test works a job through the same handler.
func NewErrorHandler(logger *slog.Logger) river.ErrorHandler {
	return &errorHandler{logger: logger}
}

func (h *errorHandler) HandleError(ctx context.Context, job *rivertype.JobRow, err error) *river.ErrorHandlerResult {
	h.logger.ErrorContext(ctx, "river job errored",
		slog.String("kind", job.Kind),
		slog.String("queue", job.Queue),
		slog.Int64("job_id", job.ID),
		slog.Int("attempt", job.Attempt),
		slog.Int("max_attempts", job.MaxAttempts),
		slog.Any("error", err),
	)
	return &river.ErrorHandlerResult{}
}

func (h *errorHandler) HandlePanic(ctx context.Context, job *rivertype.JobRow, panicVal any, trace string) *river.ErrorHandlerResult {
	h.logger.ErrorContext(ctx, "river job panicked",
		slog.String("kind", job.Kind),
		slog.String("queue", job.Queue),
		slog.Int64("job_id", job.ID),
		slog.Int("attempt", job.Attempt),
		slog.Any("panic", panicVal),
		slog.String("trace", trace),
	)
	return &river.ErrorHandlerResult{}
}
