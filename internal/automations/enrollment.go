package automations

import (
	"context"
	"errors"
	"slices"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/lifecycle"
)

// Enrollment rules (GLOSSARY: Enrollment). A Contact is enrolled into an Automation
// at most once, ever: a second Enroll, after the first run completed, exited or
// failed, creates nothing. These are package functions rather than Module methods
// because callers already hold the Workspace's scoped client, often bound to a
// transaction (the unsubscribe path exits inside the bus's).
//
// Making an Enrollment and ending Enrollments on an exit or goal Event (EndOnEvent) are ordered
// per Contact: both lock the Contact's row first (lockContact) and hold it to the end of their
// transaction. Without it, at READ COMMITTED, an Enrollment written but not yet committed is
// invisible to an exit or goal Event handled meanwhile, and that Event, stored after the
// Enrollment looked for it, would be missed by both: the Enrollment would walk its steps after the
// payment it was waiting for.

// Event is an Event the Automations react to, as its trigger job hands it on (#751, #752): its
// action, the id its row is stored under (events.Envelope.SourceID; empty in a job of a release
// before #752) and when it occurred.
type Event struct {
	Action     string
	SourceID   string
	OccurredAt time.Time
}

// Origin is what an Enrollment keeps of the Event it began on: when it occurred (began_at), and its
// action and properties, which every step binds as `event` (#752). An Origin with only OccurredAt
// is an Enrollment no Event began: its `event` is empty.
type Origin struct {
	Action     string
	OccurredAt time.Time
	Properties map[string]any
}

// OriginOf is the Origin of an Event: its action and occurred_at, and the properties of the row
// stored with its source id, bounded as a Rule sees them (events.BoundedProperties). The row is
// stored before the trigger job is queued (#751); a job without a source id, or an Event no longer
// stored (Retention or an Erasure removed it), gives no properties.
func OriginOf(ctx context.Context, s *ent.Scoped, ev Event) (Origin, error) {
	o := Origin{Action: ev.Action, OccurredAt: ev.OccurredAt}
	stored, err := storedEvent(ctx, s, ev.SourceID)
	if err != nil || stored == nil {
		return o, err
	}
	o.Properties = events.BoundedProperties(stored.Properties)
	return o, nil
}

// storedEvent is the Event stored with sourceID, nil when there is none.
func storedEvent(ctx context.Context, s *ent.Scoped, sourceID string) (*ent.Event, error) {
	if sourceID == "" {
		return nil, nil
	}
	stored, err := s.Event().Query().Where(event.SourceID(sourceID)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	return stored, err
}

// Enroll starts an Enrollment of the Contact in the Automation that no Event began: it begins now
// and its `event` is empty. It returns its run id. runID is 0, and nothing is made, when the
// Contact has been enrolled before (the common trigger-replay path) or another request won the
// race. enrolled says that a step is to run: it is false too for an Enrollment made already ended
// (enroll). The Contact's lock is held to the end of the transaction s is bound to.
func Enroll(ctx context.Context, s *ent.Scoped, automationID, contactID int64) (runID int64, enrolled bool, err error) {
	return enroll(ctx, s, automationID, contactID, Origin{OccurredAt: time.Now()}, admitsAll)
}

// EnrollOnRule enrolls the Contact as Enroll does, for a Rule that fired on the Event stored with
// sourceID (the Firing's source_id): the Enrollment began when that Event occurred and keeps its
// action and properties, as the Rule's own http Action binds them (#752). When no such Event is
// stored any more (Retention or an Erasure removed it), it begins now and its `event` is empty.
func EnrollOnRule(ctx context.Context, s *ent.Scoped, automationID, contactID int64, sourceID string) (runID int64, enrolled bool, err error) {
	fired, err := storedEvent(ctx, s, sourceID)
	if err != nil {
		return 0, false, err
	}
	o := Origin{OccurredAt: time.Now()}
	if fired != nil {
		o.Action, o.Properties = fired.Action, events.BoundedProperties(fired.Properties)
		if fired.OccurredAt != nil {
			o.OccurredAt = *fired.OccurredAt
		}
	}
	return enroll(ctx, s, automationID, contactID, o, admitsAll)
}

// EnrollOnTrigger enrolls the Contact as Enroll does, for the Event of the Origin (OriginOf): the
// Enrollment began when it occurred (#751) and keeps its action and properties (#752). The trigger
// consumer picks the Automations before it enrolls, so a Publish that moved the Trigger, a
// Deactivate or an Archive may have landed since: under the lock it enrolls nobody (runID 0) unless
// the Automation is still kept, active and started by the action in its live version.
func EnrollOnTrigger(ctx context.Context, s *ent.Scoped, automationID, contactID int64, o Origin) (runID int64, enrolled bool, err error) {
	return enroll(ctx, s, automationID, contactID, o, func(a *ent.Automation) bool {
		live := a.Edges.LiveVersion
		return a.DiscardedAt == nil && a.Status == automation.StatusActive && live != nil && live.TriggerEvent == o.Action
	})
}

func admitsAll(*ent.Automation) bool { return true }

// enroll makes the Enrollment that began on the Event of o when admits, called under the share lock
// with the Automation and its live version, accepts it, and copies o's action and properties onto it
// (#752). An exit or goal Event of the live version recorded for the Contact since o occurred may
// have been handled before the Enrollment existed, with nothing to end then: the Enrollment is made
// already ended by the first of them (exited or goal_reached, so it counts and the Contact stays
// enrolled once ever), and no step is to run.
func enroll(ctx context.Context, s *ent.Scoped, automationID, contactID int64, o Origin, admits func(*ent.Automation) bool) (runID int64, enrolled bool, err error) {
	locked, err := lockContact(ctx, s, contactID)
	if err != nil {
		return 0, false, err
	}
	if !locked {
		// Another Workspace's Contact is not in this one: the reference check the Create below
		// would make says so, so keep that answer.
		return 0, false, ent.ErrNotInWorkspace
	}
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
	ending, ended, err := endedSince(ctx, s, a.Edges.LiveVersion, contactID, o.OccurredAt)
	if err != nil {
		return 0, false, err
	}
	create := s.AutomationRun().Create().
		SetAutomationID(automationID).
		SetVersionID(*a.LiveVersionID).
		SetContactID(contactID).
		SetBeganAt(o.OccurredAt)
	if o.Action != "" {
		create.SetEventAction(o.Action)
	}
	if len(o.Properties) > 0 {
		create.SetEventProperties(o.Properties)
	}
	run, err := create.Save(ctx)
	if ent.IsConstraintError(err) {
		return 0, false, nil // lost an enrollment race
	}
	if err != nil {
		return 0, false, err
	}
	if !ended {
		return run.ID, true, nil
	}
	if err := move(ctx, s, run.ID, ending, nil); err != nil {
		return 0, false, err
	}
	return run.ID, false, nil
}

// lockContact locks the Contact's row to the end of the transaction s is bound to: the one point
// that orders the making of the Contact's Enrollments against the exit and goal Events that end
// them. The lock is FOR NO KEY UPDATE, not FOR UPDATE: a row that references the Contact (an Event
// stored for it, the Outbound message of a step's send) takes the key-share lock, which this one
// leaves free, so neither an Events consumer nor a send in flight under the fence ever waits for
// it. false when the Contact is not in the Workspace.
func lockContact(ctx context.Context, s *ent.Scoped, contactID int64) (bool, error) {
	q := s.Contact().Query().Where(contact.ID(contactID))
	q.Modify(func(sel *sql.Selector) { sel.For(sql.LockNoKeyUpdate) })
	ids, err := q.IDs(ctx)
	return len(ids) > 0, err
}

// endedSince is the end the first exit or goal Event of the version recorded for the Contact at
// since or later makes, if there is one. Like the trigger consumer it leaves out loaded history and
// a machine open or click (ADR 0053); an exit and the goal at the same moment are taken in the
// order they were recorded.
func endedSince(ctx context.Context, s *ent.Scoped, v *ent.AutomationVersion, contactID int64, since time.Time) (trigger, bool, error) {
	actions := slices.Clone(v.ExitEvents)
	if v.GoalEvent != nil {
		actions = append(actions, *v.GoalEvent)
	}
	if len(actions) == 0 {
		return "", false, nil
	}
	first, err := s.Event().Query().
		Where(
			event.ContactID(contactID),
			event.Or(lo.Map(actions, func(action string, _ int) predicate.Event { return byHuman(action) })...),
			event.OccurredAtGTE(since),
			event.Backfill(false),
		).
		Order(ent.Asc(event.FieldOccurredAt), ent.Asc(event.FieldID)).
		First(ctx)
	if ent.IsNotFound(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	t, ends := endingBy(v, first.Action)
	return t, ends, nil
}

// byHuman matches the Events with the action, leaving out an open or a click a machine made (ADR
// 0053).
func byHuman(action string) predicate.Event {
	if events.IsEngagement(action) {
		return event.And(event.Action(action), events.HumanOnly)
	}
	return event.Action(action)
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

// EndOnEvent ends the Contact's active Enrollments that an Event with the action ends (#751): one
// whose version names the action as its goal event reaches its goal (`goal_reached`), one whose
// version lists it among its exit events leaves early (`exited`). Each Enrollment reads the version
// it stands on, whatever the Automation's status or live version now. Only an Event that occurred
// when the Enrollment began (the triggering Event's occurred_at) or later ends it, so loaded history
// does not end a fresh Enrollment, and an Event that came after the trigger but was handled before
// the Enrollment was made still does.
// It matches by Contact alone, and an Enrollment another writer moved first is left alone. It locks
// the Contact first, as making an Enrollment does, so s is bound to a transaction: an Enrollment
// being made meanwhile is either committed before the active ones are read, or made after this
// transaction and finds the Event among the stored ones.
func EndOnEvent(ctx context.Context, s *ent.Scoped, contactID int64, action string, occurredAt time.Time) error {
	locked, err := lockContact(ctx, s, contactID)
	if err != nil || !locked {
		return err // a Contact erased since has no Enrollment left to end
	}
	runs, err := s.AutomationRun().Query().
		Where(automationrun.ContactID(contactID), automationrun.StatusEQ(automationrun.StatusActive)).
		WithVersion().
		All(ctx)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if occurredAt.Before(run.BeganAt) {
			continue
		}
		t, ends := endingBy(run.Edges.Version, action)
		if !ends {
			continue
		}
		err := move(ctx, s, run.ID, t, nil)
		if errors.Is(err, lifecycle.ErrInvalidTransition) {
			continue // another writer moved it out of active first
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// endingBy is the move an Event with the action makes on an Enrollment standing on the version:
// its goal, one of its exits, or none.
func endingBy(v *ent.AutomationVersion, action string) (trigger, bool) {
	switch {
	case v.GoalEvent != nil && *v.GoalEvent == action:
		return triggerReachGoal, true
	case slices.Contains(v.ExitEvents, action):
		return triggerExit, true
	}
	return "", false
}

// TriggeredBy lists the active Automations whose live version an Event with the action enrolls
// into (ADR 0065).
func TriggeredBy(ctx context.Context, s *ent.Scoped, action string) ([]int64, error) {
	return s.Automation().Query().
		Where(
			automation.StatusEQ(automation.StatusActive),
			automation.HasLiveVersionWith(automationversion.TriggerEvent(action)),
		).
		IDs(ctx)
}
