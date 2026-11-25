package automations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationsteprecord"
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

// dueOf is when the Step st is due for an Enrollment that reaches it at now, computed once, when it
// is reached, and kept (ADR 0124, ADR 0126): a wait's delay after now, and a Wait until's moment in
// the Contact's zone, or else the Workspace's, which zone reads only for a Wait until; nil for any
// other Step, which runs at once.
func dueOf(st *automationstep.Step, now time.Time, zone func() (*time.Location, error)) (*time.Time, error) {
	switch p := st.Payload.(type) {
	case automationstep.Wait:
		due := now.Add(time.Duration(p.Seconds) * time.Second)
		return &due, nil
	case automationstep.WaitUntil:
		loc, err := zone()
		if err != nil {
			return nil, err
		}
		due, err := untilDue(p.Until, now, loc)
		if err != nil {
			return nil, fmt.Errorf("wait until step %s: %w", st.ID, err)
		}
		return &due, nil
	default:
		return nil, nil
	}
}

// untilDue is the moment a Wait until's moment comes at or after now in loc: one equal to now waits
// zero (decision 11 of #1613).
func untilDue(u automationstep.Until, now time.Time, loc *time.Location) (time.Time, error) {
	switch u := u.(type) {
	case automationstep.TimeOfDay:
		w, err := u.Calendar()
		if err != nil {
			return time.Time{}, err
		}
		return w.Next(now, loc), nil
	case nil:
		return time.Time{}, errors.New("a wait until without a moment")
	}
	return time.Time{}, fmt.Errorf("the moment %T has no due moment", u)
}

// contactZone is the zone the local times of the Contact c are read in (localcalendar.Zone): its
// own time zone when the zone database loads it, else the Workspace time zone, UTC while unset.
func contactZone(ctx context.Context, s *ent.Scoped, c *ent.Contact) (*time.Location, error) {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	return localcalendar.Zone(c.TimeZone, ws.Timezone)
}

// runZone is contactZone of the Contact of the Enrollment runID.
func runZone(ctx context.Context, s *ent.Scoped, runID int64) (*time.Location, error) {
	c, err := s.AutomationRun().Query().Where(automationrun.ID(runID)).QueryContact().Only(ctx)
	if err != nil {
		return nil, err
	}
	return contactZone(ctx, s, c)
}

// reach records that the Enrollment runID reached the Step of the id at now, due at due (a wait's
// due moment, nil for any other Step).
func reach(ctx context.Context, s *ent.Scoped, runID int64, stepID automationstep.ID, now time.Time, due *time.Time) error {
	return s.AutomationStepRecord().Create().
		SetRunID(runID).
		SetStepID(stepID).
		SetReachedAt(now).
		SetNillableDueAt(due).
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
