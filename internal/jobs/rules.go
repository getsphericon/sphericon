package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/jobkind"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/tags"
)

// EvaluateRulesArgs is the evaluation of one Event against the Workspace's Rules (ADR 0054): the
// Rules consumer queues it, like the Automations consumer queues EvaluateTriggerArgs.
type EvaluateRulesArgs struct {
	WorkspaceID int64          `json:"workspace_id"`
	ContactID   int64          `json:"contact_id"`
	Action      string         `json:"action"`
	SourceID    string         `json:"source_id"`
	RecordedAt  time.Time      `json:"recorded_at"`
	Properties  map[string]any `json:"properties,omitempty"`
}

func (EvaluateRulesArgs) Kind() string { return jobkind.RuleEvaluate.Kind }

// OnRuleEvent queues the evaluation of an Event against the Workspace's Rules. It is the seam the
// Rules consumer calls (events.RuleEvaluator).
func (c *Client) OnRuleEvent(ctx context.Context, ev events.RuleEvent) error {
	_, err := c.river.Insert(ctx, EvaluateRulesArgs{
		WorkspaceID: ev.WorkspaceID,
		ContactID:   ev.ContactID,
		Action:      ev.Action,
		SourceID:    ev.SourceID,
		RecordedAt:  ev.RecordedAt,
		Properties:  ev.Properties,
	}, nil)
	return err
}

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
// already does the thing for an Automation step; internal/rules imports none of them.
func RuleActionRunners(mod *outbound.Module, bus *events.Bus) rules.ActionRunners {
	return rules.ActionRunners{
		rules.ActionApplyTag:  tagAction{mod: mod, apply: true},
		rules.ActionRemoveTag: tagAction{mod: mod},
		rules.ActionEnroll:    enrollAction{bus: bus},
		rules.ActionHTTP:      httpAction{},
	}
}

// enrollAction enrolls the Contact in an Automation through the same Enrollment an Automation's
// trigger uses: once ever, fenced (ADR 0046). An Automation that is not active (a draft or an
// archived one) or a Contact already enrolled is Skipped, not Failed. The Enrollment and the job
// that starts stepping it commit together, so a retry never finds an enrolled Contact whose run
// never started.
type enrollAction struct {
	bus *events.Bus
}

func (a enrollAction) Run(ctx context.Context, s *ent.Scoped, in rules.ActionInput) (rules.Result, error) {
	auto, err := s.Automation().Query().Where(automation.ID(in.Action.AutomationID)).Only(ctx)
	switch {
	case ent.IsNotFound(err):
		return rules.Result{Status: rules.Skipped, Detail: "the automation is gone"}, nil
	case err != nil:
		return rules.Result{}, err
	case auto.Status != automation.StatusActive:
		return rules.Result{Status: rules.Skipped, Detail: "the automation is not active"}, nil
	}
	enrolled := false
	err = a.bus.WithinScopedEnqueueTx(ctx, s, func(ts *ent.Scoped, q events.Enqueuer) error {
		runID, ok, err := automations.Enroll(ctx, ts, auto.ID, in.ContactID)
		if err != nil || !ok {
			return err
		}
		enrolled = true
		return q.Enqueue(ctx, stepping.RunStepArgs{RunID: runID, Step: 0}, nil)
	})
	switch {
	case err != nil:
		return rules.Result{}, err
	case !enrolled:
		return rules.Result{Status: rules.Skipped, Detail: "the contact is already enrolled"}, nil
	}
	return rules.Result{Status: rules.Succeeded}, nil
}

// tagAction applies or removes a Tag. The change is the `system` actor's (ADR 0022), as an
// Automation's tag step is: a Tag the Action creates is audited.
type tagAction struct {
	mod   *outbound.Module
	apply bool
}

func (a tagAction) Run(ctx context.Context, s *ent.Scoped, in rules.ActionInput) (rules.Result, error) {
	sys := a.mod.AsSystem(s)
	var err error
	if a.apply {
		_, err = tags.New().Apply(ctx, sys, in.ContactID, in.Action.Tag)
	} else {
		err = tags.New().Remove(ctx, sys, in.ContactID, in.Action.Tag)
	}
	switch {
	case errors.Is(err, tags.ErrInvalidName), errors.Is(err, tags.ErrContactNotFound):
		return rules.Result{Status: rules.Failed, Detail: err.Error()}, nil
	case err != nil:
		return rules.Result{}, err
	}
	return rules.Result{Status: rules.Succeeded}, nil
}
