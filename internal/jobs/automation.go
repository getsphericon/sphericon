package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/jobkind"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/tags"
)

// --- trigger evaluation: enroll contacts into matching automations ---

type EvaluateTriggerArgs struct {
	WorkspaceID int64  `json:"workspace_id"`
	ContactID   int64  `json:"contact_id"`
	Action      string `json:"action"`
}

func (EvaluateTriggerArgs) Kind() string { return jobkind.EvaluateTrigger }

type EvaluateTriggerWorker struct {
	river.WorkerDefaults[EvaluateTriggerArgs]
	ent *ent.Client
}

func (w *EvaluateTriggerWorker) Work(ctx context.Context, job *river.Job[EvaluateTriggerArgs]) error {
	// Job entry point (ADR 0017): the scope is built from the loaded contact's
	// Workspace, not from the job argument.
	c, err := w.ent.Contact.Get(ctx, job.Args.ContactID)
	if ent.IsNotFound(err) {
		return nil // contact deleted since enqueue; nothing to enroll
	}
	if err != nil {
		return err
	}
	if c.WorkspaceID != job.Args.WorkspaceID {
		return nil // stale or forged job args; never enroll across Workspaces
	}
	runIDs, err := EvaluateTrigger(ctx, w.ent.Scoped(c.WorkspaceID), c.ID, job.Args.Action)
	if err != nil {
		return err
	}
	rc := river.ClientFromContext[pgx.Tx](ctx)
	for _, id := range runIDs {
		if _, err := rc.Insert(ctx, RunStepArgs{RunID: id}, nil); err != nil {
			return err
		}
	}
	return nil
}

// EvaluateTrigger enrolls a contact into every active automation in the workspace
// whose trigger_event matches action, creating one AutomationRun each (enroll-
// once-ever via the unique index). Returns the new run IDs to advance. Pure (no
// queue) so it can be tested directly.
func EvaluateTrigger(ctx context.Context, s *ent.Scoped, contactID int64, action string) ([]int64, error) {
	autos, err := s.Automation().Query().
		Where(
			automation.StatusEQ(automation.StatusActive),
			automation.TriggerEvent(action),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	var runIDs []int64
	for _, a := range autos {
		runID, enrolled, err := automations.Enroll(ctx, s, a.ID, contactID)
		if err != nil {
			return nil, err
		}
		if !enrolled {
			continue
		}
		runIDs = append(runIDs, runID)
	}
	return runIDs, nil
}

// --- run stepping: walk the contact through the automation's steps ---

type RunStepArgs struct {
	RunID int64 `json:"run_id"`
}

func (RunStepArgs) Kind() string { return jobkind.RunStep }

type RunStepWorker struct {
	river.WorkerDefaults[RunStepArgs]
	ent *ent.Client
	mod *outbound.Module
}

func (w *RunStepWorker) Work(ctx context.Context, job *river.Job[RunStepArgs]) error {
	res, err := RunStep(ctx, w.ent, w.mod, job.Args.RunID)
	if err != nil {
		return err
	}
	if res.Done {
		return nil
	}
	rc := river.ClientFromContext[pgx.Tx](ctx)
	if res.Enqueue != nil {
		// The Enrollment is parked: the job inserted here moves it on (or fails it).
		_, err = rc.Insert(ctx, res.Enqueue, nil)
		return err
	}
	opts := &river.InsertOpts{}
	if res.ResumeAt != nil {
		opts.ScheduledAt = *res.ResumeAt
	}
	_, err = rc.Insert(ctx, RunStepArgs{RunID: job.Args.RunID}, opts)
	return err
}

// StepResult tells the worker how to schedule the next step. Done = the run is
// finished. ResumeAt set = schedule the next step then (a wait); nil = run the
// next step immediately. Enqueue set = the Enrollment is parked on that job (an HTTP step,
// which runs on a queue of its own): it moves the run on, so no next step is scheduled.
type StepResult struct {
	Done     bool
	ResumeAt *time.Time
	Enqueue  river.JobArgs
}

// RunStep executes the run's current step and advances it. Exported and queue-free
// so a test can drive a whole automation by looping until Done. An email step is
// one Outbound send under this automation's Sending source (ADR 0015): the module
// owns eligibility, the unsubscribe footer and headers, and the send record, and
// RunStep only turns its Outcome into the enrollment's next state.
func RunStep(ctx context.Context, client *ent.Client, mod *outbound.Module, runID int64) (StepResult, error) {
	run, err := client.AutomationRun.Get(ctx, runID)
	if ent.IsNotFound(err) {
		return StepResult{Done: true}, nil // erased with its Contact (ADR 0021) after the job was queued
	}
	if err != nil {
		return StepResult{}, fmt.Errorf("load run %d: %w", runID, err)
	}
	// Job entry point: the scoped client is built from the loaded row's Workspace
	// (ADR 0017), then handed to the modules.
	scoped := client.Scoped(run.WorkspaceID)

	a, err := scoped.Automation().Get(ctx, run.AutomationID)
	if err != nil {
		return StepResult{}, err
	}
	steps, err := automations.Decode(a.Definition)
	if err != nil {
		return failRun(ctx, scoped, run.ID, fmt.Errorf("automation %d definition: %w", a.ID, err))
	}

	if run.CurrentStep >= len(steps) {
		return completeRun(ctx, scoped, run.ID)
	}

	switch s := steps[run.CurrentStep]; s.Type {
	case automations.StepWait:
		resume := time.Now().Add(time.Duration(s.Seconds) * time.Second)
		if _, err := scoped.AutomationRun().UpdateOneID(run.ID).SetCurrentStep(run.CurrentStep + 1).SetResumeAt(resume).Save(ctx); err != nil {
			return StepResult{}, err
		}
		return StepResult{ResumeAt: &resume}, nil

	case automations.StepApplyTag, automations.StepRemoveTag:
		// A tag step changes the Contact's Tags and moves straight on; it sends nothing.
		// The change is the `system` actor's (ADR 0022): a Tag the step creates is audited.
		sys := mod.AsSystem(scoped)
		var err error
		if s.Type == automations.StepApplyTag {
			_, err = tags.New().Apply(ctx, sys, run.ContactID, s.Tag)
		} else {
			err = tags.New().Remove(ctx, sys, run.ContactID, s.Tag)
		}
		if err != nil {
			return StepResult{}, err
		}
		if _, err := scoped.AutomationRun().UpdateOneID(run.ID).SetCurrentStep(run.CurrentStep + 1).ClearResumeAt().Save(ctx); err != nil {
			return StepResult{}, err
		}
		return StepResult{}, nil // continue immediately

	case automations.StepHTTP:
		// The call runs on its own queue (ADR 0039); this job's work ends here.
		return StepResult{Enqueue: RunHTTPStepArgs{RunID: run.ID, Step: run.CurrentStep}}, nil

	case automations.StepEmail:
		c, err := scoped.Contact().Get(ctx, run.ContactID)
		if err != nil {
			return StepResult{}, err
		}
		// Email channel: a Contact with no email address can't receive this step.
		// Not an opt-out — the sequence simply completes.
		if c.Email == nil {
			return completeRun(ctx, scoped, run.ID)
		}
		step := run.CurrentStep
		res, err := mod.Send(ctx, scoped, outbound.AutomationStep(a, run, step, c, s.Subject, s.Body))
		if err != nil {
			return StepResult{}, err // retryable: the worker's retry replays the same key
		}
		// A Hold or a Deferral waits, unchanged, and asks again later (an Automation step
		// cannot snooze, so it reschedules itself).
		retry := res.Retry(0)
		if retry.Wanted {
			resume := time.Now().Add(retry.After)
			return StepResult{ResumeAt: &resume}, nil
		}
		switch res.Outcome {
		case outbound.Sent:
			// Advance past the email step. A crash between the send and this write
			// replays the recorded Sent on retry, so the step never sends twice.
			if _, err := scoped.AutomationRun().UpdateOneID(run.ID).SetCurrentStep(step + 1).ClearResumeAt().Save(ctx); err != nil {
				return StepResult{}, err
			}
			return StepResult{}, nil // continue immediately
		case outbound.Skipped:
			// An ineligible destination (suppressed, or unsubscribed from this
			// automation / from everything) exits the enrollment — a run never
			// silently keeps walking steps while skipping every email.
			if err := automations.ExitRun(ctx, scoped, run.ID); !settled(err) {
				return StepResult{}, err
			}
			return StepResult{Done: true}, nil
		default: // outbound.Failed
			return failRun(ctx, scoped, run.ID, fmt.Errorf("email step %d: %s", step, res.Reason))
		}

	default:
		return failRun(ctx, scoped, run.ID, fmt.Errorf("unknown step type %q", s.Type))
	}
}

// settled reports whether a state write left nothing to report: it succeeded, or the
// Enrollment had already left `active` (an exit won while the step ran) or was erased with
// its Contact, so the late step finishes quietly. Any other error is returned.
func settled(err error) bool {
	return err == nil || errors.Is(err, lifecycle.ErrInvalidTransition) || ent.IsNotFound(err)
}

// completeRun ends the Enrollment `completed`; a run that is no longer active stays as it is.
func completeRun(ctx context.Context, s *ent.Scoped, runID int64) (StepResult, error) {
	if err := automations.Complete(ctx, s, runID); !settled(err) {
		return StepResult{}, err
	}
	return StepResult{Done: true}, nil
}

// failRun ends the Enrollment `failed` and returns cause for the job, or nothing when the
// run had already left `active`. A failed write is returned with the cause, never dropped.
func failRun(ctx context.Context, s *ent.Scoped, runID int64, cause error) (StepResult, error) {
	err := automations.Fail(ctx, s, runID, automations.Failure{})
	if errors.Is(err, lifecycle.ErrInvalidTransition) || ent.IsNotFound(err) {
		return StepResult{Done: true}, nil
	}
	if err != nil {
		return StepResult{}, errors.Join(cause, fmt.Errorf("fail run %d: %w", runID, err))
	}
	return StepResult{}, cause
}
