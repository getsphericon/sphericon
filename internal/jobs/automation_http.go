package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/webhook"
)

// RunHTTPStepWorker POSTs an HTTP step's rendered body and moves the Enrollment on, or ends
// it `failed` with the reason when the retries are spent.
type RunHTTPStepWorker struct {
	river.WorkerDefaults[stepping.RunHTTPStepArgs]

	ent    *ent.Client
	cipher *secrets.Cipher
	step   *stepping.Module
	client webhook.Doer
	now    func() time.Time
	fence  automations.Fencer
}

// NewRunHTTPStepWorker builds the worker that runs an Automation's HTTP step through doer, on the
// clock now, behind the Enrollment's fence.
func NewRunHTTPStepWorker(client *ent.Client, fence automations.Fencer, cipher *secrets.Cipher, step *stepping.Module, doer webhook.Doer, now func() time.Time) *RunHTTPStepWorker {
	return &RunHTTPStepWorker{ent: client, cipher: cipher, step: step, client: doer, now: now, fence: fence}
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
	// The lifecycle table decides: a job that finds the Enrollment exited makes no call, tags
	// nothing and queues nothing.
	if active, err := automations.IsActive(ctx, s, run.ID); err != nil {
		return err
	} else if !active {
		return nil
	}
	if run.CurrentStep != job.Args.Step {
		return nil // the Enrollment moved on without this job
	}

	version, err := s.AutomationVersion().Get(ctx, run.VersionID)
	if err != nil {
		return err
	}
	steps, err := automations.Decode(version.Steps)
	if err != nil {
		return w.fail(ctx, run, automations.Failure{Kind: automationrun.FailureKindInvalidBody}, river.JobCancel(err))
	}
	if run.CurrentStep >= len(steps) || steps[run.CurrentStep].Type != automations.StepHTTP {
		return nil // the Enrollment moved on since the job was queued
	}
	step := steps[run.CurrentStep]

	c, err := s.Contact().Get(ctx, run.ContactID)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	opened, err := automations.OpenHTTP(w.cipher, step)
	if err != nil {
		return err
	}
	body, err := automations.RenderBody(step.Body, c, run)
	if err != nil {
		// A render that is not valid JSON would fail the same way on every attempt.
		return w.fail(ctx, run, automations.Failure{Kind: automationrun.FailureKindInvalidBody}, river.JobCancel(err))
	}

	// The call is fenced against an exit (Fencer): it starts only on an Enrollment that
	// is still active and an exit waits for it, so nothing is posted after an exit.
	sendErr := w.fence(ctx, s, run.ID, func(ctx context.Context) error {
		return webhook.Post(ctx, w.client, webhook.Call{
			URL: opened.URL, Secret: opened.Secret, Headers: opened.Headers, Body: []byte(body),
			ID: fmt.Sprintf("automation_%d_%d", run.ID, run.CurrentStep),
		})
	})
	if automations.Stale(sendErr) {
		return nil // an exit won before the call: nothing was sent
	}
	if sendErr == nil {
		return w.finish(ctx, run)
	}

	now := w.now()
	f := automations.FailureOfDelivery(sendErr)
	if verdict, limited := webhook.DecideRateLimited(sendErr, job.CreatedAt, now); limited {
		if verdict.GiveUp {
			return w.fail(ctx, run, f, river.JobCancel(sendErr)) // a receiver that never lets up
		}
		return river.JobSnooze(verdict.Wait) // never an attempt
	}
	if !webhook.Retryable(sendErr) {
		return w.fail(ctx, run, f, river.JobCancel(sendErr)) // no retry opens a blocked address or turns a 4xx into a 2xx
	}
	if job.Attempt >= job.MaxAttempts {
		return w.fail(ctx, run, f, sendErr)
	}
	return sendErr
}

// NextRetry is the call's retry policy (webhook.HookNextRetry): days, as for a Webhook delivery.
func (w *RunHTTPStepWorker) NextRetry(job *river.Job[stepping.RunHTTPStepArgs]) time.Time {
	if job.AttemptedAt == nil {
		return time.Time{} // river falls back to its default
	}
	return webhook.HookNextRetry(*job.AttemptedAt, job.Attempt)
}

// finish moves the Enrollment past the HTTP step and queues the step after it, in one
// transaction; an Enrollment an exit took meanwhile, or one a previous attempt already moved,
// is left as it is.
func (w *RunHTTPStepWorker) finish(ctx context.Context, run *ent.AutomationRun) error {
	return w.step.FinishHTTPStep(ctx, w.ent.Scoped(run.WorkspaceID), run.ID, run.CurrentStep)
}

// fail ends the Enrollment `failed` with the reason and returns result for the job.
func (w *RunHTTPStepWorker) fail(ctx context.Context, run *ent.AutomationRun, f automations.Failure, result error) error {
	if err := w.step.FailHTTPStep(ctx, w.ent.Scoped(run.WorkspaceID), run.ID, f); err != nil {
		return err
	}
	return result
}
