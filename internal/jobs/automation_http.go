package jobs

import (
	"context"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
)

// RunHTTPStepWorker establishes Workspace scope and translates HTTP Step execution decisions
// into the queue's retry, cancellation and snooze actions.
type RunHTTPStepWorker struct {
	hookRetries[automations.RunHTTPStepArgs]

	ent  *ent.Client
	step *automations.StepExecution
}

// NewRunHTTPStepWorker builds the queue adapter for the Step execution's HTTP execution, spreading
// its retries by jitter.
func NewRunHTTPStepWorker(client *ent.Client, step *automations.StepExecution, jitter webhook.HookJitter) *RunHTTPStepWorker {
	return &RunHTTPStepWorker{jitter: jitter, ent: client, step: step}
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
	decision, err := w.step.ExecuteHTTP(ctx, s, automations.ExecuteHTTPInput{RunID: run.ID, Step: job.Args.Step, Attempt: hookAttempt(job.JobRow)})
	if err != nil {
		return err
	}
	return executionResult(decision)
}
