package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/jobkind"
	"github.com/getsphericon/sphericon/internal/stepping"
)

// --- trigger evaluation: enroll contacts into matching automations ---

type EvaluateTriggerArgs struct {
	WorkspaceID int64  `json:"workspace_id"`
	ContactID   int64  `json:"contact_id"`
	Action      string `json:"action"`
	// SourceAutomationID is the Automation whose email the Event came from, 0 for any other Event.
	SourceAutomationID int64 `json:"source_automation_id,omitempty"`
	// OccurredAt is when the Event happened: it ends only the Enrollments that began by then, and
	// an Enrollment it makes begins then. Zero in a job of a release before #751 (stepping.OnEvent).
	OccurredAt time.Time `json:"occurred_at"`
}

func (EvaluateTriggerArgs) Kind() string { return jobkind.EvaluateTrigger.Kind }

type EvaluateTriggerWorker struct {
	river.WorkerDefaults[EvaluateTriggerArgs]

	ent *ent.Client
	bus *events.Bus
}

// NewEvaluateTriggerWorker builds the worker that hands an Event of a Contact to the Automations
// (stepping.OnEvent): the Enrollments it ends and the ones it makes.
func NewEvaluateTriggerWorker(client *ent.Client, bus *events.Bus) *EvaluateTriggerWorker {
	return &EvaluateTriggerWorker{ent: client, bus: bus}
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
	args := job.Args
	return stepping.OnEvent(ctx, w.bus, w.ent.Scoped(c.WorkspaceID), c.ID, args.Action, args.SourceAutomationID, args.OccurredAt)
}

// --- run stepping: the adapter of the Enrollment stepping module (internal/stepping) ---

// RunStepWorker executes one step of one Enrollment.
type RunStepWorker struct {
	river.WorkerDefaults[stepping.RunStepArgs]

	ent *ent.Client
	mod *stepping.Module
}

// NewRunStepWorker builds the worker over the raw client the job entry point scopes from.
func NewRunStepWorker(client *ent.Client, mod *stepping.Module) *RunStepWorker {
	return &RunStepWorker{ent: client, mod: mod}
}

func (w *RunStepWorker) Work(ctx context.Context, job *river.Job[stepping.RunStepArgs]) error {
	// Job entry point (ADR 0017): the scope is built from the loaded Enrollment's Workspace.
	// One erased with its Contact after the job was queued (ADR 0021) ends the job quietly.
	run, err := w.ent.AutomationRun.Get(ctx, job.Args.RunID)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return w.mod.Step(ctx, w.ent.Scoped(run.WorkspaceID), run.ID, job.Args.Step)
}
