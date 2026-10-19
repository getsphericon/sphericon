package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/jobkind"
	"github.com/getsphericon/sphericon/internal/webhook"
)

// QueueHTTPSteps isolates the HTTP steps of Automations (ADR 0039): an outbound call to a
// third party can take its whole timeout and be retried for days, so it must neither hold
// the default queue's automation steps nor the Webhook deliveries' queue.
const QueueHTTPSteps = "http_steps"

// RunHTTPStepArgs is one HTTP step of one Enrollment. The args carry the run id, so Erasure
// can delete a queued call by it (ADR 0021): renaming `run_id` breaks that purge.
type RunHTTPStepArgs struct {
	RunID int64 `json:"run_id"`
	Step  int   `json:"step"`
}

func (RunHTTPStepArgs) Kind() string { return jobkind.HTTPStep }

// InsertOpts puts the call on its own queue with the retry horizon of a hook (HookMaxAttempts).
func (RunHTTPStepArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueHTTPSteps, MaxAttempts: HookMaxAttempts()}
}

// RunHTTPStepWorker POSTs an HTTP step's rendered body and moves the Enrollment on, or ends
// it `failed` with the reason when the retries are spent.
type RunHTTPStepWorker struct {
	river.WorkerDefaults[RunHTTPStepArgs]
	ent    *ent.Client
	mod    *automations.Module
	client webhook.Doer
	now    func() time.Time
}

func (w *RunHTTPStepWorker) Work(ctx context.Context, job *river.Job[RunHTTPStepArgs]) error {
	// Job entry point: the Workspace is read off the loaded run (ADR 0017). A run or Contact
	// erased after the job was queued (ADR 0021) ends the job quietly, and nothing is sent.
	run, err := w.ent.AutomationRun.Get(ctx, job.Args.RunID)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if run.Status != automationrun.StatusActive {
		return nil
	}
	s := w.ent.Scoped(run.WorkspaceID)
	switch run.CurrentStep {
	case job.Args.Step + 1:
		return w.continueRun(ctx, run, false) // the call was made, the follow-up was not queued
	case job.Args.Step:
	default:
		return nil // the Enrollment moved on without this job
	}

	a, err := s.Automation().Get(ctx, run.AutomationID)
	if err != nil {
		return err
	}
	steps, err := automations.Decode(a.Definition)
	if err != nil {
		return w.fail(ctx, run, failure{kind: automationrun.FailureKindInvalidBody}, river.JobCancel(err))
	}
	if run.CurrentStep >= len(steps) || steps[run.CurrentStep].Type != automations.StepHTTP {
		return nil // the Automation was edited since the job was queued
	}
	step := steps[run.CurrentStep]

	c, err := s.Contact().Get(ctx, run.ContactID)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	opened, err := w.mod.OpenHTTP(step)
	if err != nil {
		return err
	}
	body, err := automations.RenderBody(step.Body, c)
	if err != nil {
		// A render that is not valid JSON would fail the same way on every attempt.
		return w.fail(ctx, run, failure{kind: automationrun.FailureKindInvalidBody}, river.JobCancel(err))
	}

	sendErr := webhook.Post(ctx, w.client, webhook.Call{
		URL: opened.URL, Secret: opened.Secret, Headers: opened.Headers, Body: []byte(body),
		ID: fmt.Sprintf("automation_%d_%d", run.ID, run.CurrentStep),
	})
	if sendErr == nil {
		return w.continueRun(ctx, run, true)
	}

	now := w.now()
	var delivery *webhook.DeliveryError
	if !errors.As(sendErr, &delivery) {
		delivery = &webhook.DeliveryError{Kind: webhook.KindNetwork}
	}
	f := failure{kind: automationrun.FailureKind(delivery.Kind), excerpt: delivery.Excerpt}
	if delivery.Kind == webhook.KindHTTPStatus {
		f.status = delivery.Status
	}
	if limited := rateLimit(sendErr); limited != nil {
		if now.Sub(job.CreatedAt) >= HookRetryHorizon {
			return w.fail(ctx, run, f, river.JobCancel(sendErr)) // a receiver that never lets up
		}
		return river.JobSnooze(HookSnooze(limited.RetryAfter, now)) // never an attempt
	}
	if delivery.Kind == webhook.KindBlockedAddress {
		return w.fail(ctx, run, f, river.JobCancel(sendErr)) // no retry opens a blocked address
	}
	if job.Attempt >= job.MaxAttempts {
		return w.fail(ctx, run, f, sendErr)
	}
	return sendErr
}

// NextRetry is the call's retry policy (HookNextRetry): days, as for a Webhook delivery.
func (w *RunHTTPStepWorker) NextRetry(job *river.Job[RunHTTPStepArgs]) time.Time {
	if job.AttemptedAt == nil {
		return time.Time{} // river falls back to its default
	}
	return HookNextRetry(*job.AttemptedAt, job.Attempt)
}

// continueRun queues the Enrollment's next step, first moving it past the HTTP step unless a
// previous attempt of this job already did.
func (w *RunHTTPStepWorker) continueRun(ctx context.Context, run *ent.AutomationRun, advance bool) error {
	if advance {
		err := w.ent.Scoped(run.WorkspaceID).AutomationRun().UpdateOneID(run.ID).
			SetCurrentStep(run.CurrentStep + 1).ClearResumeAt().Exec(ctx)
		if err != nil {
			return err
		}
	}
	_, err := river.ClientFromContext[pgx.Tx](ctx).Insert(ctx, RunStepArgs{RunID: run.ID}, nil)
	return err
}

// failure is what an Enrollment keeps of a failed HTTP step.
type failure struct {
	kind    automationrun.FailureKind
	status  int
	excerpt string
}

// fail ends the Enrollment `failed` with the reason and returns result for the job.
func (w *RunHTTPStepWorker) fail(ctx context.Context, run *ent.AutomationRun, f failure, result error) error {
	u := w.ent.Scoped(run.WorkspaceID).AutomationRun().UpdateOneID(run.ID).
		SetStatus(automationrun.StatusFailed).ClearResumeAt().SetFailureKind(f.kind)
	if f.status != 0 {
		u.SetFailureStatus(f.status)
	}
	if f.excerpt != "" {
		u.SetFailureExcerpt(f.excerpt)
	}
	if err := u.Exec(ctx); err != nil {
		return err
	}
	return result
}
