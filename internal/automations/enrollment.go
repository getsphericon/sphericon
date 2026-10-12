package automations

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
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
	run, err := s.AutomationRun().Create().
		SetAutomationID(automationID).
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
// absent one is left alone.
func Exit(ctx context.Context, s *ent.Scoped, automationID, contactID int64) (bool, error) {
	n, err := s.AutomationRun().Update().
		Where(
			automationrun.AutomationID(automationID),
			automationrun.ContactID(contactID),
			automationrun.StatusEQ(automationrun.StatusActive),
		).
		SetStatus(automationrun.StatusExited).
		ClearResumeAt().
		Save(ctx)
	return n > 0, err
}

// ExitRun ends one Enrollment by run id, for the step executor that already holds
// the run (an ineligible destination exits it rather than walk on skipping mail).
func ExitRun(ctx context.Context, s *ent.Scoped, runID int64) error {
	return s.AutomationRun().UpdateOneID(runID).
		SetStatus(automationrun.StatusExited).
		ClearResumeAt().
		Exec(ctx)
}
