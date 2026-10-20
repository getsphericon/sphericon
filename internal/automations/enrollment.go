package automations

import (
	"context"
	"errors"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationversion"
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
	// Check first so the common "already enrolled" path does not trip the unique
	// index (a violation would poison a surrounding transaction); the index stays
	// as the race safety net.
	exists, err := s.AutomationRun().Query().
		Where(automationrun.AutomationID(automationID), automationrun.ContactID(contactID)).
		Exist(ctx)
	if err != nil || exists {
		return 0, false, err
	}
	// The Enrollment begins on the Automation's latest version and stays on it (ADR 0049).
	versionID, err := s.AutomationVersion().Query().
		Where(automationversion.AutomationID(automationID)).
		Order(ent.Desc(automationversion.FieldVersion)).
		FirstID(ctx)
	if ent.IsNotFound(err) {
		// Another Workspace's Automation has no version in this one: the reference check
		// the Create below would make says so, so keep that answer.
		if _, getErr := s.Automation().Get(ctx, automationID); ent.IsNotFound(getErr) {
			return 0, false, ent.ErrNotInWorkspace
		}
		return 0, false, ErrNoVersion
	}
	if err != nil {
		return 0, false, err
	}
	run, err := s.AutomationRun().Create().
		SetAutomationID(automationID).
		SetVersionID(versionID).
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
