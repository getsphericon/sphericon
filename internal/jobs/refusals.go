package jobs

import (
	"context"
	"errors"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The job runner holds no domain rule (ADR 0107): whether an error ends a job or retries it is
// decided by its form alone. A refusal of a module (a validation refusal) is the job's final
// outcome, since the same input is refused again; any other error retries the job.

// refused reports whether err is a refusal of a module: a validation.RefusalError or a
// validation.FieldsError in its chain.
func refused(err error) bool {
	if _, ok := errors.AsType[*validation.RefusalError](err); ok {
		return true
	}
	_, ok := errors.AsType[*validation.FieldsError](err)
	return ok
}

// RefusalsEnd is the worker middleware that ends a job its worker refused for good: a refusal is
// cancelled (river.JobCancel), so it is not retried, and any other error goes on to river's retry.
type RefusalsEnd struct {
	river.MiddlewareDefaults
}

var _ rivertype.WorkerMiddleware = (*RefusalsEnd)(nil)

func (*RefusalsEnd) Work(ctx context.Context, _ *rivertype.JobRow, doInner func(context.Context) error) error {
	err := doInner(ctx)
	if err != nil && refused(err) {
		return river.JobCancel(err)
	}
	return err
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
