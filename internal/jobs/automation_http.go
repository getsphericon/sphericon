package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
)

// RunHTTPStepWorker establishes Workspace scope and translates HTTP Step execution decisions
// into the queue's retry, cancellation and snooze actions.
type RunHTTPStepWorker struct {
	river.WorkerDefaults[automations.RunHTTPStepArgs]

	ent  *ent.Client
	step *automations.StepExecution
}

// NewRunHTTPStepWorker builds the queue adapter for the Step execution's HTTP execution.
func NewRunHTTPStepWorker(client *ent.Client, step *automations.StepExecution) *RunHTTPStepWorker {
	return &RunHTTPStepWorker{ent: client, step: step}
}

func (w *RunHTTPStepWorker) Work(ctx context.Context, job *river.Job[automations.RunHTTPStepArgs]) error {
	// Job entry point: the Workspace is read off the loaded run (ADR 0017). A run or Contact
	// erased after the job was queued (ADR 0021) ends the job quietly, and nothing is sent.
	run, err := w.ent.AutomationRun.Get(ctx, job.Args.RunID)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	s := w.ent.Scoped(run.WorkspaceID)
	decision, err := w.step.ExecuteHTTP(ctx, s, automations.ExecuteHTTPInput{RunID: run.ID, Step: job.Args.Step, Attempt: httpcall.Attempt{
		Number: job.Attempt, Limit: job.MaxAttempts, CreatedAt: job.CreatedAt,
	}})
	if err != nil {
		return err
	}
	if decision.Wait > 0 {
		return river.JobSnooze(decision.Wait)
	}
	if decision.Cancel != nil {
		return river.JobCancel(decision.Cancel)
	}
	return decision.Retry
}

// NextRetry is the call's retry policy (webhook.HookNextRetry): days, as for a Webhook delivery.
func (w *RunHTTPStepWorker) NextRetry(job *river.Job[automations.RunHTTPStepArgs]) time.Time {
	if job.AttemptedAt == nil {
		return time.Time{} // river falls back to its default
	}
	return webhook.HookNextRetry(*job.AttemptedAt, job.Attempt)
}
