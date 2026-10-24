package rules

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/rule"
	"github.com/getsphericon/sphericon/internal/events"
)

// DryRunOutcome is whether a stored Event would fire a Rule, and if not, why.
type DryRunOutcome string

const (
	// WouldFire: the Event's action is a Trigger event and the Condition holds.
	WouldFire DryRunOutcome = "would_fire"
	// NotTriggered: the Event's action is none of the Rule's Trigger events.
	NotTriggered DryRunOutcome = "not_triggered"
	// Machine: software made the Event (a mail scanner's open or click, ADR 0053), and an Event
	// like that never triggers a Rule.
	Machine DryRunOutcome = "machine"
	// Backfill: the Event was loaded as history, and history never triggers a Rule.
	Backfill DryRunOutcome = "backfill"
	// NoContact: the Event is about no Contact, so a Rule never reacts to it.
	NoContact DryRunOutcome = "no_contact"
	// ConditionNotMet: the Condition does not hold for the Contact right now.
	ConditionNotMet DryRunOutcome = "condition_not_met"
)

// PlannedAction is one Action a Rule would run, described for a person: its type and a one-line
// summary. A new Action kind needs only a case in Summary; an unknown one shows its type.
type PlannedAction struct {
	Type    ActionType
	Summary string
}

// DryRun is what a stored Event would do to a Rule. Actions are the ones that would run, in
// order, and empty unless the Outcome is WouldFire.
type DryRun struct {
	Outcome DryRunOutcome
	Actions []PlannedAction
}

// Summary describes an Action in one line, without running it.
func (a Action) Summary() string {
	switch a.Type {
	case ActionApplyTag:
		return fmt.Sprintf("Apply tag %q", a.Tag)
	case ActionRemoveTag:
		return fmt.Sprintf("Remove tag %q", a.Tag)
	case ActionEnroll:
		return fmt.Sprintf("Enroll in automation %d", a.AutomationID)
	case ActionHTTP:
		// The URL is stored sealed and may carry a secret: never described here.
		return "Call an HTTP endpoint"
	default:
		return string(a.Type)
	}
}

// DryRun evaluates a stored Event against one Rule exactly as Evaluate does (the same Trigger
// event match and the same Condition evaluation, against the Contact's data now) and reports
// what would happen. It writes nothing and runs nothing: no Firing, no job, no Action. Unlike a
// real firing it ignores the Rule's status and activation time, so a draft can be tried.
func (m *Module) DryRun(ctx context.Context, s *ent.Scoped, ruleID, eventID int64) (DryRun, error) {
	r, err := m.Get(ctx, s, ruleID)
	if err != nil {
		return DryRun{}, err
	}
	stored, err := s.Event().Get(ctx, eventID)
	if ent.IsNotFound(err) {
		return DryRun{}, ErrEventNotFound
	}
	if err != nil {
		return DryRun{}, err
	}
	if stored.Backfill {
		return DryRun{Outcome: Backfill}, nil
	}
	if events.IsMachine(stored.Properties) {
		return DryRun{Outcome: Machine}, nil
	}
	if stored.ContactID == nil {
		return DryRun{Outcome: NoContact}, nil
	}
	ev := Event{ContactID: *stored.ContactID, Action: stored.Action, Properties: events.RuleProperties(stored.Properties)}

	triggered, err := s.Rule().Query().Where(rule.ID(r.ID), triggeredBy(ev.Action)).Exist(ctx)
	if err != nil {
		return DryRun{}, err
	}
	if !triggered {
		return DryRun{Outcome: NotTriggered}, nil
	}
	holds, err := m.holds(ctx, s, r, ev)
	if err != nil {
		return DryRun{}, err
	}
	if !holds {
		return DryRun{Outcome: ConditionNotMet}, nil
	}
	actions, err := DecodeActions(r.Actions)
	if err != nil {
		return DryRun{}, err
	}
	planned := make([]PlannedAction, 0, len(actions))
	for i := range actions {
		a := &actions[i]
		planned = append(planned, PlannedAction{Type: a.Type, Summary: a.Summary()})
	}
	return DryRun{Outcome: WouldFire, Actions: planned}, nil
}
