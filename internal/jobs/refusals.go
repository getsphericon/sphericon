package jobs

import (
	"context"
	"errors"
	"log/slog"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The job runner holds no domain rule (ADR 0107): whether an error ends a job or retries it is
// decided by its form alone. A refusal is the job's final outcome, since the same input is refused
// again: a module's (a validation refusal) and a provider's (messaging.ErrRefused, the same answer
// on every retry until a person changes the account). Any other error retries the job.

// refused reports whether err is a refusal of a module: a validation.RefusalError or a
// validation.FieldsError in its chain.
func refused(err error) bool {
	if _, ok := errors.AsType[*validation.RefusalError](err); ok {
		return true
	}
	_, ok := errors.AsType[*validation.FieldsError](err)
	return ok
}

// RefusalsEnd is the worker middleware that ends a job refused for good, the one place that
// decides it: a refusal is cancelled (river.JobCancel), so it is not retried, and any other error
// goes on to river's retry and its ErrorHandler. river logs a cancel only at debug level, so the
// middleware logs it: a provider's refusal as an error, since a person must change the account, and
// a module's as a warning, since the module said why.
type RefusalsEnd struct {
	river.MiddlewareDefaults
}

var _ rivertype.WorkerMiddleware = (*RefusalsEnd)(nil)

func (*RefusalsEnd) Work(ctx context.Context, job *rivertype.JobRow, doInner func(context.Context) error) error {
	err := doInner(ctx)
	providerRefused := errors.Is(err, messaging.ErrRefused)
	if err == nil || (!providerRefused && !refused(err)) {
		return err
	}
	level := slog.LevelWarn
	if providerRefused {
		level = slog.LevelError
	}
	slog.Log(ctx, level, "river job refused, cancelled",
		slog.String("kind", job.Kind),
		slog.String("queue", job.Queue),
		slog.Int64("job_id", job.ID),
		slog.Int("attempt", job.Attempt),
		slog.Any("error", err),
	)
	return river.JobCancel(err)
}

// actionResult is a Rule Action's outcome for the error its module returned: nil succeeds; a
// refusal ends the Action, Skipped when the subject's state does not allow it now (a conflict,
// such as an Automation that is not active) and Failed otherwise, with the refusal's message as
// the detail; any other error is returned, so the Actions job retries.
func actionResult(err error) (rules.Result, error) {
	if err == nil {
		return rules.Result{Status: rules.Succeeded}, nil
	}
	if !refused(err) {
		return rules.Result{}, err
	}
	status := rules.Failed
	if r, ok := errors.AsType[*validation.RefusalError](err); ok && r.Kind() == validation.KindConflict {
		status = rules.Skipped
	}
	detail, _ := validation.Shown(err)
	return rules.Result{Status: status, Detail: detail}, nil
}
