package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
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

// EvaluateRulesWorker loads the Contact and hands the Event to the Rule module, which writes a
// Firing and queues its Actions job for every Rule that fires.
type EvaluateRulesWorker struct {
	river.WorkerDefaults[EvaluateRulesArgs]

	ent *ent.Client
	mod *rules.Module
}

func (w *EvaluateRulesWorker) Work(ctx context.Context, job *river.Job[EvaluateRulesArgs]) error {
	// Job entry point (ADR 0017): the scope is built from the loaded Contact's Workspace, not
	// from the job argument.
	c, err := w.ent.Contact.Get(ctx, job.Args.ContactID)
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
func RuleActionRunners(mod *outboundmessages.Module, bus *events.Bus, steps *automations.StepExecution) rules.ActionRunners {
	tagging := contacts.New(bus)
	return rules.ActionRunners{
		rules.ActionApplyTag:  tagAction{mod: mod, tags: tagging, apply: true},
		rules.ActionRemoveTag: tagAction{mod: mod, tags: tagging},
		rules.ActionEnroll:    enrollAction{steps: steps},
		rules.ActionHTTP:      httpAction{},
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
	enrolled, err := a.steps.EnrollOnRule(ctx, s, automations.EnrollOnRuleInput{
		AutomationID: in.Action.AutomationID, ContactID: in.ContactID, SourceID: in.Firing.SourceID,
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
