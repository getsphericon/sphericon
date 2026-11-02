package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/httpcall"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/webhook"
)

// RunHTTPStepWorker establishes Workspace scope and translates HTTP Step execution decisions
// into the queue's retry, cancellation and snooze actions.
type RunHTTPStepWorker struct {
	river.WorkerDefaults[stepping.RunHTTPStepArgs]

	ent    *ent.Client
	cipher *secrets.Cipher
	step   *stepping.Module
	client webhook.Doer
	now    func() time.Time
}

// NewRunHTTPStepWorker builds the queue adapter for the stepping module's HTTP execution.
func NewRunHTTPStepWorker(client *ent.Client, cipher *secrets.Cipher, step *stepping.Module, doer webhook.Doer, now func() time.Time) *RunHTTPStepWorker {
	return &RunHTTPStepWorker{ent: client, cipher: cipher, step: step, client: doer, now: now}
}

func (w *RunHTTPStepWorker) Work(ctx context.Context, job *river.Job[stepping.RunHTTPStepArgs]) error {
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
	decision, err := w.step.ExecuteHTTP(ctx, s, run.ID, job.Args.Step, httpcall.Attempt{
		Number: job.Attempt, Limit: job.MaxAttempts, CreatedAt: job.CreatedAt,
	}, w.cipher, w.client, w.now)
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
func (w *RunHTTPStepWorker) NextRetry(job *river.Job[stepping.RunHTTPStepArgs]) time.Time {
	if job.AttemptedAt == nil {
		return time.Time{} // river falls back to its default
	}
	return webhook.HookNextRetry(*job.AttemptedAt, job.Attempt)
}
