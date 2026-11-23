package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/kernel/ruleaction"
)

// EvaluateRulesArgs is the evaluation of one Event against the Workspace's Rules (ADR 0054): the
// Rules consumer queues it, like the Automations consumer queues EvaluateTriggerArgs.
type EvaluateRulesArgs struct {
	jobkind.Workspace

	ContactID  int64          `json:"contact_id"`
	Action     string         `json:"action"`
	SourceID   string         `json:"source_id"`
	RecordedAt time.Time      `json:"recorded_at"`
	Properties map[string]any `json:"properties,omitempty"`
}

func (EvaluateRulesArgs) Kind() string { return jobkind.RuleEvaluate.Kind }

// InsertOpts puts the evaluation, a consumer's job, in the events queue (ADR 0101).
func (EvaluateRulesArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: events.QueueEvents}
}

// EvaluateRulesWorker loads the Contact and hands the Event to the Rule module, which writes a
// Firing and queues its Actions job for every Rule that fires.
type EvaluateRulesWorker struct {
	river.WorkerDefaults[EvaluateRulesArgs]

	ent *ent.Client
	mod *rules.Module
}

// NewEvaluateRulesWorker builds the worker that hands an Event of a Contact to the Rules
// (rules.Module.Evaluate) over the raw client the job entry point scopes from.
func NewEvaluateRulesWorker(client *ent.Client, mod *rules.Module) *EvaluateRulesWorker {
	return &EvaluateRulesWorker{ent: client, mod: mod}
}

func (w *EvaluateRulesWorker) Work(ctx context.Context, job *river.Job[EvaluateRulesArgs]) error {
	// Job entry point (ADR 0017): the scope is built from the loaded Contact's Workspace, not
	// from the job argument.
	// Only its Workspace is read: the module reads the Contact again in its own scope.
	c, err := w.ent.Contact.Query().Where(contact.ID(job.Args.ContactID)).Select(contact.FieldWorkspaceID).Only(ctx)
	if ent.IsNotFound(err) {
		return nil // erased since the Event was recorded: nothing to act on
	}
	if err != nil {
		return err
	}
	if c.WorkspaceID != job.Args.WorkspaceID {
		return nil // stale or forged job args; never fire across Workspaces
	}
	_, err = w.mod.Evaluate(ctx, w.ent.Scoped(c.WorkspaceID), rules.Event{
		ContactID:  c.ID,
		Action:     job.Args.Action,
		SourceID:   job.Args.SourceID,
		RecordedAt: job.Args.RecordedAt,
		Properties: job.Args.Properties,
	})
	return err
}

// RunRuleActionsWorker runs a Firing's Actions, each through the Runner of its type.
type RunRuleActionsWorker struct {
	river.WorkerDefaults[rules.ActionsArgs]

	ent     *ent.Client
	mod     *rules.Module
	runners rules.ActionRunners
}

// NewRunRuleActionsWorker builds the worker that runs a Firing's Actions (rules.Module.RunActions)
// through runners (RuleActionRunners).
func NewRunRuleActionsWorker(client *ent.Client, mod *rules.Module, runners rules.ActionRunners) *RunRuleActionsWorker {
	return &RunRuleActionsWorker{ent: client, mod: mod, runners: runners}
}

func (w *RunRuleActionsWorker) Work(ctx context.Context, job *river.Job[rules.ActionsArgs]) error {
	// Job entry point (ADR 0017): the scope is built from the Workspace of the loaded Firing.
	f, err := w.ent.RuleFiring.Get(ctx, job.Args.FiringID)
	if ent.IsNotFound(err) {
		return nil // erased with its Contact after the job was queued
	}
	if err != nil {
		return err
	}
	return w.mod.RunActions(ctx, w.ent.Scoped(f.WorkspaceID), f.ID, w.runners)
}

// RuleActionRunners are the Runners of the Actions a Rule can have. Each reuses the module that
// already does the thing for an Automation step; internal/domain/orchestration/rules imports none of them.
func RuleActionRunners(mod *outboundmessages.Module, tagging *contacts.Module, steps *automations.StepExecution) rules.ActionRunners {
	return rules.ActionRunners{
		ruleaction.ApplyTag:  tagAction{mod: mod, tags: tagging, apply: true},
		ruleaction.RemoveTag: tagAction{mod: mod, tags: tagging},
		ruleaction.Enroll:    enrollAction{steps: steps},
	}
}

// enrollAction enrolls the Contact in an Automation through the same Enrollment an Automation's
// trigger uses: once ever, fenced (ADR 0046), beginning when the Event the Rule fired on occurred
// (automations.StepExecution.EnrollOnRule). The Enrollment decides under its lock whether the
// Automation may enroll anybody; its refusal ends the Action (actionResult). A Contact already
// enrolled is Skipped, not Failed. The Enrollment and the job that starts stepping it commit
// together, so a retry never finds an enrolled Contact whose run never started.
type enrollAction struct {
	steps *automations.StepExecution
}

func (a enrollAction) Run(ctx context.Context, s *ent.Scoped, in rules.ActionInput) (rules.Result, error) {
	// The Automations the chain of the Event the Rule fired on passed through ride in the job's
	// metadata (events.JobCorrelation): the entry skips one that names the Automation (ADR 0127).
	enrolled, err := a.steps.EnrollOnRule(ctx, s, automations.EnrollOnRuleInput{
		AutomationID: in.Action.AutomationID, ContactID: in.ContactID, FiringID: in.Firing.ID, SourceID: in.Firing.SourceID,
		Chain: events.AutomationsOf(ctx),
	})
	if err == nil && !enrolled {
		return rules.Result{Status: rules.Skipped, Detail: "the contact is already enrolled"}, nil
	}
	return actionResult(err)
}

// tagAction applies or removes a Tag. The change is the `system` actor's (ADR 0022), as an
// Automation's tag step is: a Tag the Action creates is audited.
type tagAction struct {
	mod   *outboundmessages.Module
	tags  *contacts.Module
	apply bool
}

func (a tagAction) Run(ctx context.Context, s *ent.Scoped, in rules.ActionInput) (rules.Result, error) {
	sys := a.mod.AsSystem(s)
	var err error
	if a.apply {
		_, err = a.tags.ApplyTag(ctx, sys, in.ContactID, in.Action.Tag)
	} else {
		err = a.tags.RemoveTag(ctx, sys, in.ContactID, in.Action.Tag)
	}
	return actionResult(err)
}
