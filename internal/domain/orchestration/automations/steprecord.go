package automations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationsteprecord"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
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

// arrival is what a Step reads, on demand, of the Enrollment that reaches it: its Contact's
// calendar, which a Wait until reads, and the message a send Step sent it, which a Wait for event on
// an open or a click reads (heardOf).
type arrival struct {
	contact func() (automationstep.Contact, error)
	message func(automationstep.AwaitedMessage) (heard, error)
}

// arrive is what the Step st decides for an Enrollment that reaches it at now, computed once, then
// kept on its Step record (ADR 0124, ADR 0126): a wait's delay after now; a Wait for event's
// deadline, its timeout after now, or, on an open or a click, the path it already knows of
// (reachedPath); and a Wait until's moment for its Contact, in the Contact's zone or else the
// Workspace's, or why it goes on at once (automationstep.Reach). Any other Step decides nothing
// (nil) and runs at once.
func arrive(st *automationstep.Step, now time.Time, a arrival) (automationstep.Due, error) {
	switch p := st.Payload.(type) {
	case automationstep.Wait:
		return automationstep.WaitsUntil{At: now.Add(time.Duration(p.Seconds) * time.Second)}, nil
	case automationstep.WaitForEvent:
		if awaited, onMessage := p.Awaits.(automationstep.AwaitedMessage); onMessage {
			h, err := a.message(awaited)
			if err != nil {
				return nil, err
			}
			if path, decided := reachedPath(p, h); decided {
				return automationstep.GoesDown{Path: path}, nil
			}
		}
		// Its deadline, kept as its due moment: the job that runs then takes the timeout.
		return automationstep.WaitsUntil{At: now.Add(time.Duration(p.TimeoutSeconds) * time.Second)}, nil
	case automationstep.WaitUntil:
		c, err := a.contact()
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
// decided nothing on arrival, nil, a Wait until that went on at once, or a Wait for event that knew
// its path at once).
func dueAt(d automationstep.Due) *time.Time {
	switch d := d.(type) {
	case automationstep.WaitsUntil:
		return &d.At
	case automationstep.GoesOn, automationstep.GoesDown, nil:
	}
	return nil
}

// wentOn is why a Wait until let the Enrollment go on at once, nil for any other arrival.
func wentOn(d automationstep.Due) *automationstep.WentOn {
	switch d := d.(type) {
	case automationstep.GoesOn:
		return &d.Reason
	case automationstep.WaitsUntil, automationstep.GoesDown, nil:
	}
	return nil
}

// decidedPath is the path a Wait for event on an open or a click decided on arrival, nil for any
// other arrival.
func decidedPath(d automationstep.Due) *automationstep.ID {
	switch d := d.(type) {
	case automationstep.GoesDown:
		return &d.Path
	case automationstep.WaitsUntil, automationstep.GoesOn, nil:
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
// decided then: a wait's due moment, why a Wait until let it go on at once, or the path a Wait for
// event on an open or a click knew of at once.
func reach(ctx context.Context, s *ent.Scoped, runID int64, stepID automationstep.ID, now time.Time, d automationstep.Due) error {
	return s.AutomationStepRecord().Create().
		SetRunID(runID).
		SetStepID(stepID).
		SetReachedAt(now).
		SetNillableDueAt(dueAt(d)).
		SetNillableWentOn(wentOn(d)).
		SetNillableDecidedPathID(decidedPath(d)).
		Exec(ctx)
}

// leaving is how the Enrollment leaves a Step: done, skipped for a reason when skipped is set, or
// down the path of a fork when path is set (ADR 0124).
type leaving struct {
	skipped skipreason.Reason
	path    *pathTaken
}

// pathTaken is the path a fork sent the Enrollment down, by its id; at a Branch, what each path's
// Condition was for the Contact, by path id: true, false or unknown, never the values tested; and,
// at a Wait for event that arrived on an Event, the copy of that Event it keeps (ADR 0136).
type pathTaken struct {
	id       automationstep.ID
	outcomes map[automationstep.ID]conditiontree.Truth
	kept     *keptEvent
}

// keptEvent is the copy a Wait for event keeps of the Event it arrived on (ADR 0136): the Event's
// id, its action and its properties, bounded as the Enrollment's `event` is.
type keptEvent struct {
	eventID    int64
	action     string
	properties map[string]any
}

// keptOf is the copy of the stored Event e.
func keptOf(e *ent.Event) *keptEvent {
	return &keptEvent{eventID: e.ID, action: e.Action, properties: events.BoundedProperties(e.Properties)}
}

// stillKept is how, with the copy of the awaited Event it keeps taken again from the Event's row,
// read under a key-share lock in the transaction of the move (ADR 0136). A refusal of tracking that
// deleted the Event since the read that found it (ADR 0120) leaves no copy to keep, and the path is
// still taken: as if the move had committed first and the refusal had cleared its copy after, so a
// later Step reads the name as absent. The lock holds a deletion off until the move commits, and the
// refusal then finds the copy and clears it. It is taken before the Enrollment's own row is moved,
// in the order an Erasure deletes the Contact's Events and then its Enrollments.
func stillKept(ctx context.Context, s *ent.Scoped, how leaving) (leaving, error) {
	if how.path == nil || how.path.kept == nil {
		return how, nil
	}
	path := *how.path
	source, err := s.Event().Query().
		Where(event.ID(path.kept.eventID)).
		ForShare(func(o *sql.LockOptions) { o.Strength = sql.LockKeyShare }).
		Only(ctx)
	switch {
	case ent.IsNotFound(err):
		path.kept = nil
	case err != nil:
		return how, fmt.Errorf("read the awaited event %d: %w", path.kept.eventID, err)
	default:
		path.kept = keptOf(source)
	}
	how.path = &path
	return how, nil
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
			if k := how.path.kept; k != nil {
				u.SetAwaitedEventID(k.eventID).SetAwaitedAction(k.action)
				if k.properties != nil {
					u.SetAwaitedProperties(k.properties)
				}
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

// clearKept clears the copies of the Events the Enrollment runID's Wait for event Steps arrived on
// (ADR 0136), in the move that ends it: nothing reads them after, and an Enrollment's records are
// kept for its reports. The rest of each record stays.
func clearKept(ctx context.Context, s *ent.Scoped, runID int64) error {
	return clearKeptWhere(ctx, s, automationsteprecord.RunID(runID), automationsteprecord.AwaitedEventIDNotNil())
}

// clearKeptWhere clears the copy of an awaited Event that each Step record where selects keeps,
// all three of its columns together. The rest of each record stays.
func clearKeptWhere(ctx context.Context, s *ent.Scoped, where ...predicate.AutomationStepRecord) error {
	return s.AutomationStepRecord().Update().
		Where(where...).
		ClearAwaitedEventID().
		ClearAwaitedAction().
		ClearAwaitedProperties().
		Exec(ctx)
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
