package automations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationsteprecord"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
)

// A Step record is what an Enrollment keeps of one Step it reached (ADR 0124). The move that puts
// the Enrollment on a Step writes its record (reach), and the move that takes it off stamps it
// (leave), each in the transaction of that move and after the compare-and-set on the
// Enrollment's cursor, so a move that lost a race writes no record.

var _ = db.Declare(
	db.Fault("automation_step_records_workspace_id_run_id_step_id",
		"a Step record is written by the move that puts the Enrollment on the Step, which its cursor's compare-and-set lets happen once, and a version's Steps have no cycles"),
)

// arrive is what the Step st decides for an Enrollment that reaches it at now, computed once, then
// kept on its Step record (ADR 0124, ADR 0126): a wait's delay after now, and a Wait until's moment
// for its Contact, in the Contact's zone or else the Workspace's, or why it goes on at once
// (automationstep.Reach), which contact reads only for a Wait until. Any other Step decides
// nothing (nil) and runs at once.
func arrive(st *automationstep.Step, now time.Time, contact func() (automationstep.Contact, error)) (automationstep.Due, error) {
	switch p := st.Payload.(type) {
	case automationstep.Wait:
		return automationstep.WaitsUntil{At: now.Add(time.Duration(p.Seconds) * time.Second)}, nil
	case automationstep.WaitUntil:
		c, err := contact()
		if err != nil {
			return nil, err
		}
		reached, err := automationstep.Reach(p.Until, c, now)
		if err != nil {
			return nil, fmt.Errorf("wait until step %s: %w", st.ID, err)
		}
		return reached, nil
	default:
		return nil, nil
	}
}

// dueAt is the moment a wait holds the Enrollment until, nil for a Step it does not hold (one that
// decided nothing on arrival, nil, or a Wait until that went on at once).
func dueAt(d automationstep.Due) *time.Time {
	switch d := d.(type) {
	case automationstep.WaitsUntil:
		return &d.At
	case automationstep.GoesOn, nil:
	}
	return nil
}

// wentOn is why a Wait until let the Enrollment go on at once, nil for any other arrival.
func wentOn(d automationstep.Due) *automationstep.WentOn {
	switch d := d.(type) {
	case automationstep.GoesOn:
		return &d.Reason
	case automationstep.WaitsUntil, nil:
	}
	return nil
}

// calendarOf is what a Wait until reads of the Contact c in its Workspace (contactCalendar).
func calendarOf(ctx context.Context, s *ent.Scoped, c *ent.Contact) (automationstep.Contact, error) {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return automationstep.Contact{}, err
	}
	return contactCalendar(c, ws.Timezone)
}

// contactCalendar is what a date or a time of day reads of the Contact c in a Workspace whose time
// zone is workspaceZone (ADR 0126): the zone its local times are read in (localcalendar.Zone), its
// own time zone when the zone database loads it, else the Workspace's, UTC while unset; the
// Workspace's zone, which a datetime value's date is read in; and the dates it holds. An error is a
// Workspace zone the zone database does not load. The date scheduler and a Wait until both read a
// Contact through it.
func contactCalendar(c *ent.Contact, workspaceZone *string) (automationstep.Contact, error) {
	zone, err := localcalendar.Zone(c.TimeZone, workspaceZone)
	if err != nil {
		return automationstep.Contact{}, err
	}
	workspace, err := localcalendar.Zone(nil, workspaceZone)
	if err != nil {
		return automationstep.Contact{}, err
	}
	return automationstep.Contact{Zone: zone, Workspace: workspace, Dates: contacts.DatesOf(c)}, nil
}

// runCalendar is calendarOf the Contact of the Enrollment runID.
func runCalendar(ctx context.Context, s *ent.Scoped, runID int64) (automationstep.Contact, error) {
	c, err := s.AutomationRun().Query().Where(automationrun.ID(runID)).QueryContact().Only(ctx)
	if err != nil {
		return automationstep.Contact{}, err
	}
	return calendarOf(ctx, s, c)
}

// reach records that the Enrollment runID reached the Step of the id at now, with what the Step
// decided then: a wait's due moment, or why a Wait until let it go on at once.
func reach(ctx context.Context, s *ent.Scoped, runID int64, stepID automationstep.ID, now time.Time, d automationstep.Due) error {
	return s.AutomationStepRecord().Create().
		SetRunID(runID).
		SetStepID(stepID).
		SetReachedAt(now).
		SetNillableDueAt(dueAt(d)).
		SetNillableWentOn(wentOn(d)).
		Exec(ctx)
}

// leaving is how the Enrollment leaves a Step: done, skipped for a reason when skipped is set, or
// down the path of a fork when path is set (ADR 0124).
type leaving struct {
	skipped skipreason.Reason
	path    *pathTaken
}

// pathTaken is the path a fork sent the Enrollment down, by its id, and, at a Branch, what each
// path's Condition was for the Contact, by path id: true, false or unknown, never the values
// tested.
type pathTaken struct {
	id       automationstep.ID
	outcomes map[automationstep.ID]conditiontree.Truth
}

// leave records that the Enrollment runID left the Step of the id at now, as how says: skipped for
// its reason, down its path, or done. The left moment is a latch (ADR 0109), written once with the
// outcome; the caller has moved the Enrollment off the Step, so a record already left, or none, is
// a broken invariant, not a lost race.
func leave(ctx context.Context, s *ent.Scoped, runID int64, stepID automationstep.ID, now time.Time, how leaving) error {
	err := lifecycle.Latch(ctx, func(ctx context.Context) (bool, error) {
		u := s.AutomationStepRecord().Update().
			Where(
				automationsteprecord.RunID(runID),
				automationsteprecord.StepID(stepID),
				automationsteprecord.LeftAtIsNil(),
				automationsteprecord.OutcomeIsNil(),
			).
			SetLeftAt(now).
			SetOutcome(automationsteprecord.OutcomeDone)
		switch {
		case how.skipped != "":
			u.SetOutcome(automationsteprecord.OutcomeSkipped).SetSkipReason(how.skipped)
		case how.path != nil:
			u.SetOutcome(automationsteprecord.OutcomePath).SetPathID(how.path.id)
			if how.path.outcomes != nil {
				u.SetPathOutcomes(how.path.outcomes)
			}
		}
		n, err := u.Save(ctx)
		return n > 0, err
	})
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return fmt.Errorf("leave step %s of run %d: the Enrollment has no open record of it", stepID, runID)
	}
	return err
}

// leaveOnEnd records that the Enrollment runID, ended at endedAt through an exit, its goal or a
// failure, left the Step it stood on then, with no outcome: its end says why. An Enrollment made
// already ended, or one of a version of no Steps, stands on no Step it reached, so it has no open
// record and nothing is left.
func leaveOnEnd(ctx context.Context, s *ent.Scoped, runID int64, endedAt time.Time) error {
	err := lifecycle.Latch(ctx, func(ctx context.Context) (bool, error) {
		n, err := s.AutomationStepRecord().Update().
			Where(automationsteprecord.RunID(runID), automationsteprecord.LeftAtIsNil()).
			SetLeftAt(endedAt).
			Save(ctx)
		return n > 0, err
	})
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return nil
	}
	return err
}
