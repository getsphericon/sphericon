package jobs

import (
	"context"
	"errors"
	"log/slog"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/getsphericon/sphericon/internal/messaging"
)

// errorHandler is river's centralized error/panic sink. Without it, a job that
// exhausts its retries is discarded silently and a worker panic only reaches
// river's default logger — both invisible in production. It logs each with
// enough context (kind, attempt, queue) to alert on, and is the natural seam for
// wiring Sentry later. It cancels a job only for a provider's refusal
// (messaging.ErrRefused); every other failure follows river's retry schedule.
type errorHandler struct {
	logger *slog.Logger
}

var _ river.ErrorHandler = (*errorHandler)(nil)

// NewErrorHandler builds river's error handler: the one the client runs with, exported so a
// test works a job through the same handler.
func NewErrorHandler(logger *slog.Logger) river.ErrorHandler {
	return &errorHandler{logger: logger}
}

func (h *errorHandler) HandleError(_ context.Context, job *rivertype.JobRow, err error) *river.ErrorHandlerResult {
	// A provider's refusal is the same answer on every retry until a person changes the account,
	// so the job is cancelled instead of burning its attempts and being discarded. A job that
	// returns river.JobCancel itself never reaches this handler and is logged only at debug level,
	// which is why the cancel is decided here, where it is logged as an error.
	refused := errors.Is(err, messaging.ErrRefused)
	h.logger.Error("river job errored",
		slog.String("kind", job.Kind),
		slog.String("queue", job.Queue),
		slog.Int64("job_id", job.ID),
		slog.Int("attempt", job.Attempt),
		slog.Int("max_attempts", job.MaxAttempts),
		slog.Bool("cancelled", refused),
		slog.Any("error", err),
	)
	return &river.ErrorHandlerResult{SetCancelled: refused}
}

func (h *errorHandler) HandlePanic(_ context.Context, job *rivertype.JobRow, panicVal any, trace string) *river.ErrorHandlerResult {
	h.logger.Error("river job panicked",
		slog.String("kind", job.Kind),
		slog.String("queue", job.Queue),
		slog.Int64("job_id", job.ID),
		slog.Int("attempt", job.Attempt),
		slog.Any("panic", panicVal),
		slog.String("trace", trace),
	)
	return &river.ErrorHandlerResult{}
}
