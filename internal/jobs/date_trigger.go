package jobs

import (
	"context"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/kernel/jobextension"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

// --- the date scheduler (ADR 0126): a tick every minute, a job per Automation ---

// DateTickArgs is the scheduler's tick: every minute it queues the decision of each active
// Automation whose live version has a date Trigger.
type DateTickArgs struct{}

func (DateTickArgs) Kind() string { return "automation_date_tick" }

// DateTickWorker lists the due Automations across Workspaces and queues one DecideOccurrences job
// per Automation, as the Sending domains' re-check fans out.
type DateTickWorker struct {
	river.WorkerDefaults[DateTickArgs]

	ent         *ent.Client
	automations *automations.Module
}

// NewDateTickWorker builds the tick over the raw client the cross-Workspace read takes.
func NewDateTickWorker(client *ent.Client, module *automations.Module) *DateTickWorker {
	return &DateTickWorker{ent: client, automations: module}
}

func (w *DateTickWorker) Work(ctx context.Context, _ *river.Job[DateTickArgs]) error {
	due, err := w.automations.DateDue(ctx, w.ent)
	if err != nil {
		return err
	}
	return jobextension.QueueEach(ctx, due, func(a *ent.Automation) river.JobArgs {
		return DecideOccurrencesArgs{WorkspaceID: a.WorkspaceID, AutomationID: a.ID}
	})
}

// DecideOccurrencesArgs is the decision of one Automation's occurrences up to now.
type DecideOccurrencesArgs struct {
	jobkind.Workspace

	AutomationID int64 `json:"automation_id"`
}

func (DecideOccurrencesArgs) Kind() string { return "automation_decide_occurrences" }

// InsertOpts queues one decision of an Automation at a time (jobkind.QueuedOnce): a tick that comes
// round while the last decision waits or runs queues none.
func (DecideOccurrencesArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: jobkind.QueuedOnce()}
}

// DecideOccurrencesWorker decides one Automation's occurrences (automations.StepExecution.
// DecideOccurrences).
type DecideOccurrencesWorker struct {
	river.WorkerDefaults[DecideOccurrencesArgs]

	ent   *ent.Client
	steps *automations.StepExecution
}

// NewDecideOccurrencesWorker builds the worker over the raw client the job entry point scopes from.
func NewDecideOccurrencesWorker(client *ent.Client, steps *automations.StepExecution) *DecideOccurrencesWorker {
	return &DecideOccurrencesWorker{ent: client, steps: steps}
}

func (w *DecideOccurrencesWorker) Work(ctx context.Context, job *river.Job[DecideOccurrencesArgs]) error {
	// Job entry point (ADR 0017): the scope is built from the loaded Automation's Workspace.
	a, err := w.ent.Automation.Get(ctx, job.Args.AutomationID)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if a.WorkspaceID != job.Args.WorkspaceID {
		return nil // stale or forged job args; never decide across Workspaces
	}
	return w.steps.DecideOccurrences(ctx, w.ent.Scoped(a.WorkspaceID), a.ID)
}
