package jobs

import (
	"context"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
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
}

func (EvaluateTriggerArgs) Kind() string { return jobkind.EvaluateTrigger.Kind }

type EvaluateTriggerWorker struct {
	river.WorkerDefaults[EvaluateTriggerArgs]

	ent *ent.Client
	bus *events.Bus
}

// NewEvaluateTriggerWorker builds the worker that enrolls a Contact into the Automations an action triggers.
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
	return EvaluateTrigger(ctx, w.bus, w.ent.Scoped(c.WorkspaceID), c.ID, job.Args.Action, job.Args.SourceAutomationID)
}

// EvaluateTrigger enrolls a contact into every active automation in the workspace
// whose trigger_event matches action: one AutomationRun each (enroll-once-ever via the
// unique index), committed together with the job that starts stepping it (stepping.Enroll), one
// transaction per Automation. A Contact already enrolled keeps its run and its job. An Event
// from an Automation's own email (sourceAutomationID) never enrolls the Contact into that
// Automation, so an open-triggered Automation cannot feed itself (ADR 0053).
func EvaluateTrigger(ctx context.Context, bus *events.Bus, s *ent.Scoped, contactID int64, action string, sourceAutomationID int64) error {
	autos, err := s.Automation().Query().
		Where(
			automation.StatusEQ(automation.StatusActive),
			automation.TriggerEvent(action),
		).
		All(ctx)
	if err != nil {
		return err
	}
	for _, a := range autos {
		if a.ID == sourceAutomationID {
			continue
		}
		if _, err := stepping.Enroll(ctx, bus, s, a.ID, contactID); err != nil {
			return err
		}
	}
	return nil
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
