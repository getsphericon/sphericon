package automations

import (
	"context"
	"errors"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/lifecycle"
)

// Enrollment rules (GLOSSARY: Enrollment). A Contact is enrolled into an Automation
// at most once, ever: a second Enroll, after the first run completed, exited or
// failed, creates nothing. These are package functions rather than Module methods
// because callers already hold the Workspace's scoped client, often bound to a
// transaction (the unsubscribe path exits inside the bus's).

// Enroll starts an Enrollment of the Contact in the Automation and returns its run
// id. enrolled is false when the Contact has been enrolled before (the common
// trigger-replay path) or another request won the race; nothing is created then.
func Enroll(ctx context.Context, s *ent.Scoped, automationID, contactID int64) (runID int64, enrolled bool, err error) {
	return enroll(ctx, s, automationID, contactID, func(*ent.Automation) bool { return true })
}

// EnrollOnTrigger enrolls the Contact as Enroll does, for an Event with the action. The trigger
// consumer picks the Automations before it enrolls, so a Publish that moved the Trigger, a
// Deactivate or an Archive may have landed since: under the lock it enrolls nobody (enrolled
// false) unless the Automation is still kept, active and started by the action in its live version.
func EnrollOnTrigger(ctx context.Context, s *ent.Scoped, automationID, contactID int64, action string) (runID int64, enrolled bool, err error) {
	return enroll(ctx, s, automationID, contactID, func(a *ent.Automation) bool {
		live := a.Edges.LiveVersion
		return a.DiscardedAt == nil && a.Status == automation.StatusActive && live != nil && live.TriggerEvent == action
	})
}

// enroll starts the Enrollment when admits, called under the share lock with the Automation and
// its live version, accepts it.
func enroll(ctx context.Context, s *ent.Scoped, automationID, contactID int64, admits func(*ent.Automation) bool) (runID int64, enrolled bool, err error) {
	// Check first so the common "already enrolled" path does not trip the unique
	// index (a violation would poison a surrounding transaction); the index stays
	// as the race safety net.
	exists, err := s.AutomationRun().Query().
		Where(automationrun.AutomationID(automationID), automationrun.ContactID(contactID)).
		Exist(ctx)
	if err != nil || exists {
		return 0, false, err
	}
	// Share-lock the Automation row and read its live version under the lock, so a Publish (it
	// locks the row exclusively) is either finished before the version is read or waits for this
	// Enrollment: the Enrollment never begins on a version that is no longer live.
	a, err := s.Automation().QueryWithDiscarded().Where(automation.ID(automationID)).WithLiveVersion().ForShare().Only(ctx)
	if ent.IsNotFound(err) {
		// Another Workspace's Automation is not in this one: the reference check the Create
		// below would make says so, so keep that answer.
		return 0, false, ent.ErrNotInWorkspace
	}
	if err != nil {
		return 0, false, err
	}
	// The Enrollment begins on the Automation's live version and stays on it (ADR 0049, ADR 0065).
	if a.LiveVersionID == nil {
		return 0, false, ErrNoVersion
	}
	if !admits(a) {
		return 0, false, nil
	}
	run, err := s.AutomationRun().Create().
		SetAutomationID(automationID).
		SetVersionID(*a.LiveVersionID).
		SetContactID(contactID).
		Save(ctx)
	if ent.IsConstraintError(err) {
		return 0, false, nil // lost an enrollment race
	}
	if err != nil {
		return 0, false, err
	}
	return run.ID, true, nil
}

// Exit ends the Contact's active Enrollment in the Automation (status exited, no
// pending resume). It reports whether an active Enrollment was ended; a finished or
// absent one is left alone, and so is one another writer moved first.
func Exit(ctx context.Context, s *ent.Scoped, automationID, contactID int64) (bool, error) {
	runID, err := s.AutomationRun().Query().
		Where(
			automationrun.AutomationID(automationID),
			automationrun.ContactID(contactID),
			automationrun.StatusEQ(automationrun.StatusActive),
		).
		OnlyID(ctx)
	if ent.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	err = ExitRun(ctx, s, runID)
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return false, nil // another writer moved it out of active first
	}
	return err == nil, err
}

// ExitRun ends one Enrollment by run id, for the step executor that already holds
// the run (an ineligible destination exits it rather than walk on skipping mail).
// lifecycle.ErrInvalidTransition when it is no longer active.
func ExitRun(ctx context.Context, s *ent.Scoped, runID int64) error {
	return move(ctx, s, runID, triggerExit, nil)
}
