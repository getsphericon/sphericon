package automations

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationskippedentry"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/schema"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactlock"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// Enrollment rules (GLOSSARY: Enrollment). A Contact has at most one active Enrollment in an
// Automation, or one per value of the version's Enrollment key (ADR 0165), and whether it is
// enrolled again after one ended is the Entry policy of the version
// it would begin on (ADR 0127): `once` enrolls it at most once, ever, `again` once more after each
// end and the cooldown. The entry checks (entry.go) decide under the Contact's lock and record why
// they refuse. These are package functions rather than Module methods because callers already hold
// the Workspace's scoped client, often bound to a transaction (the unsubscribe path exits inside
// the bus's).
//
// Making an Enrollment and ending Enrollments on an exit or goal Event (endOnEvent) are ordered
// per Contact: both lock the Contact's row first (lockContact) and hold it to the end of their
// transaction. Without it, at READ COMMITTED, an Enrollment written but not yet committed is
// invisible to an exit or goal Event handled meanwhile, and that Event, stored after the
// Enrollment looked for it, would be missed by both: the Enrollment would walk its steps after the
// payment it was waiting for.
//
// Lock order: the Contact, then the Enrollment (contactlock). Every transaction that writes both an
// Enrollment's row and its Contact's row takes the Contact's lock before it touches the
// Enrollment: making one, ending them on an Event, a tag step, whose change of the Contact's Tags
// updates the Contact's row, and an Erasure, which deletes both (#811). Taken the other way round,
// such a transaction and an exit of the same Contact wait for each other and Postgres aborts one of
// them. A move that writes no Contact row (a wait, an email or HTTP step, a completion, a failure)
// takes no Contact lock.
//
// endOnEvent keeps the Contact's lock while it waits for an in-flight send's fence (FenceOn) on an
// Enrollment it ends, so a write of that Contact row (an identify upsert, an /api update) waits up
// to the send's length, which the provider's and the job's timeouts bound. That is deliberate:
// releasing the Contact before the end would reopen the race above (an Enrollment made meanwhile
// missing the Event that ends it), and the send path writes no Contact row, so the wait cannot
// close a cycle Postgres does not see.

// Event is an Event the Automations react to, as its trigger job hands it on (#751, #752): its
// action, the id its row is stored under (events.Envelope.SourceID) and when it occurred.
type Event struct {
	Action     string
	SourceID   string
	OccurredAt time.Time
	// SendLocked marks an Event a write through a Connection without mcp:send published
	// (events.Envelope.SendLocked): it ends Enrollments, but enrolls no one (ADR 0132).
	SendLocked bool
	// Chain is the Automations the chain that caused the Event passed through
	// (events.Envelope.Automations, ADR 0127): none of them enrolls the Contact on it (`cycle`).
	Chain []int64
}

// origin is what an Enrollment keeps of the Event it began on: when it occurred (began_at), and its
// action and properties, which every step binds as `event` (#752). An origin with only OccurredAt
// is an Enrollment no Event began: its `event` is empty. Cause names what caused the entry, so that
// one cause enrolls a Contact in an Automation once (ADR 0127). Chain is the Automations the chain
// that caused the entry passed through (ADR 0127): the entry checks skip an entry into one of them
// (`cycle`), and the Enrollment made keeps them with its own. BegunOnSourceID is the source id of
// the stored Event the Enrollment begins on, the trigger's or the one the Rule fired on, empty when
// none is stored: that Event never ends the Enrollment (ADR 0133), and the Enrollment key's value
// is read from it (ADR 0165), into KeyValue once the version the entry is judged on is known (nil
// when the Event lacks the property).
type origin struct {
	Action          string
	OccurredAt      time.Time
	Properties      map[string]any
	Cause           cause
	Chain           []int64
	BegunOnSourceID string
	KeyValue        *string
}

// originOf is the origin of an Event: its action and occurred_at, and the properties of the row
// stored with its source id, bounded as a Rule sees them (events.BoundedProperties). The row is
// stored before the trigger job is queued (#751); an Event no longer stored (Retention or an Erasure
// removed it) gives no properties.
func originOf(ctx context.Context, s *ent.Scoped, ev Event) (origin, error) {
	o := origin{Action: ev.Action, OccurredAt: ev.OccurredAt, Cause: eventCause(ev.SourceID), Chain: ev.Chain}
	stored, err := storedEvent(ctx, s, ev.SourceID)
	if err != nil || stored == nil {
		return o, err
	}
	o.Properties, o.BegunOnSourceID = events.BoundedProperties(stored.Properties), ev.SourceID
	return o, nil
}

// storedEvent is the Event stored with sourceID, nil when there is none.
func storedEvent(ctx context.Context, s *ent.Scoped, sourceID string) (*ent.Event, error) {
	stored, err := s.Event().Query().Where(event.SourceID(sourceID)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	return stored, err
}

// entered is what an entry made: the Enrollment, by its run id (0 when none was made), whether a
// step is to run, and the reason the entry checks skipped the entry (none when they let it through).
type entered struct {
	runID    int64
	stepping bool
	skipped  automationskippedentry.Reason
}

// enrollOnRule enrolls the Contact (enroll) for the Firing of a Rule that fired on the Event stored
// with in.SourceID (the Firing's source_id): the Enrollment began when that Event occurred and keeps its
// action and properties, as the Rule's own http Action binds them (#752). When no such Event is
// stored any more (Retention or an Erasure removed it), it begins now and its `event` is empty.
// The Rule job reads the Automation before it enrolls, so a Deactivate or an Archive may have
// landed since: under the lock it enrolls nobody and returns ErrInactive unless the Automation is
// still kept and active (ADR 0099).
func (m *StepExecution) enrollOnRule(ctx context.Context, s *ent.Scoped, pub events.Publisher, in EnrollOnRuleInput, now time.Time) (entered, error) {
	automationID, contactID := in.AutomationID, in.ContactID
	fired, err := storedEvent(ctx, s, in.SourceID)
	if err != nil {
		return entered{}, err
	}
	o := origin{OccurredAt: now, Cause: firingCause(in.FiringID), Chain: in.Chain}
	if fired != nil {
		o.Action, o.Properties, o.OccurredAt = fired.Action, events.BoundedProperties(fired.Properties), fired.OccurredAt
		o.BegunOnSourceID = in.SourceID
	}
	return m.enroll(ctx, s, pub, automationID, contactID, o, func(a *ent.Automation, _ *ent.Contact, _ *origin) (bool, error) {
		if !active(a) {
			return false, ErrInactive
		}
		return true, nil
	})
}

// enrollOnTrigger enrolls the Contact (enroll) for the Event of the origin (originOf): the
// Enrollment began when it occurred (#751) and keeps its action and properties (#752). The trigger
// consumer picks the Automations before it enrolls, so a Publish that moved the Trigger, a
// Deactivate or an Archive may have landed since: under the lock it enrolls nobody (no run id) unless
// the Automation is still kept, active and its live version's Trigger matches the Event, its
// action and its filter (matchesEvent): a filter miss is the first entry check, no entry and no
// skipped entry (ADR 0133).
func (m *StepExecution) enrollOnTrigger(ctx context.Context, s *ent.Scoped, pub events.Publisher, automationID, contactID int64, o origin) (entered, error) {
	return m.enroll(ctx, s, pub, automationID, contactID, o, func(a *ent.Automation, _ *ent.Contact, _ *origin) (bool, error) {
		live := a.Edges.LiveVersion
		if !active(a) || live == nil || live.TriggerAction != o.Action {
			return false, nil
		}
		// The first entry check: an Event its filter misses is no entry, and nothing is recorded.
		return matchesEvent(ctx, s, eventMatcherOf(live), o.Action, event.SourceID(sourceOf(o.Cause)))
	})
}

// active is whether the Automation is kept and active, so it may enroll anybody.
func active(a *ent.Automation) bool {
	return a.ArchivedAt == nil && a.Status == automation.StatusActive
}

// The unique constraints of the Automation's tables (ADR 0106): no client can break them.
var _ = db.Declare(
	db.Fault(schema.ActiveEnrollmentIndex,
		"an Enrollment is made under the Contact's lock once the entry checks found none of the Contact's active in the Automation"),
	db.Fault("automation_runs_workspace_automation_contact_ordinal",
		"an Enrollment's ordinal is the next after the Contact's Enrollments in the Automation, read under the Contact's lock"),
	db.Fault("automation_versions_workspace_id_automation_id_version",
		"a version's number is the next after the newest, read under the Workspace lock (versioneddefinition)"),
)

// enroll makes the Enrollment that began on the Event of o when admits, called under the share lock
// with the Automation and its live version and under the Contact's lock with the Contact, accepts
// it (it may set when the entry begins, o.OccurredAt, from what it reads there), and the entry checks let it through
// (judgeEntry), and copies o's action and properties onto it (#752). A live version with an
// Enrollment key reads the key's value from o's stored Event (keyValueOf) before the checks judge
// the entry, and the Enrollment keeps it (ADR 0165). admits answers whether the
// Enrollment is made: a refusal with an error returns that error, one without makes nothing and
// returns none. An entry the checks refuse makes nothing and is recorded as skipped, with its
// reason (entered.skipped). An exit or goal Event of the live version recorded for the Contact since o occurred
// may have been handled before the Enrollment existed, with nothing to end then: the Enrollment is
// made already ended by the first of them (exited or goal_reached, so it counts for the ordinal, the
// cooldown and `once`), and no step is to run. It returns what the entry made: no run id, and
// nothing is made, when the entry is refused or its Event or Firing made an Enrollment already (an
// entry handled again). The Contact's lock is held to the end of the transaction s is
// bound to.
func (m *StepExecution) enroll(ctx context.Context, s *ent.Scoped, pub events.Publisher, automationID, contactID int64, o origin, admits func(*ent.Automation, *ent.Contact, *origin) (bool, error)) (entered, error) {
	held, locked, err := m.lockContact(ctx, s, contactID)
	if err != nil {
		return entered{}, err
	}
	if !locked {
		// Another Workspace's Contact is not in this one: the reference check the Create below
		// would make says so, so keep that answer.
		return entered{}, ent.ErrNotInWorkspace
	}
	a, err := shareLive(ctx, s, automationID)
	if ent.IsNotFound(err) {
		// Another Workspace's Automation is not in this one: the reference check the Create
		// below would make says so, so keep that answer.
		return entered{}, ent.ErrNotInWorkspace
	}
	if err != nil {
		return entered{}, err
	}
	admitted, err := admits(a.row, held.Contact(), &o)
	if err != nil || !admitted {
		return entered{}, err
	}
	if key := a.row.Edges.LiveVersion.EnrollmentKey; key != nil {
		keyValue, err := keyValueOf(ctx, s, o.BegunOnSourceID, *key)
		if err != nil {
			return entered{}, err
		}
		o.KeyValue = keyValue
	}
	judged, err := judgeEntry(ctx, s, a.row.Edges.LiveVersion, contactID, o)
	if err != nil {
		return entered{}, err
	}
	if judged.skipped {
		return entered{skipped: judged.reason}, recordSkip(ctx, s, automationID, contactID, o, judged.reason)
	}
	ending, endedAt, ended, err := endedSince(ctx, s, a.row.Edges.LiveVersion, contactID, o)
	if err != nil {
		return entered{}, err
	}
	// The Enrollment reaches its first Step as it is made; a wait there is due from now (ADR 0124).
	now := m.now()
	first, stepping := automationstep.First(a.row.Edges.LiveVersion.Steps)
	var arrived automationstep.Due
	if stepping && !ended {
		arrived, err = arrive(first, now, func() (automationstep.Contact, error) { return calendarOf(ctx, s, held.Contact()) })
		if err != nil {
			return entered{}, err
		}
	}
	runID, inserted, err := begin(ctx, s, held, a, o, judged.ordinal, dueAt(arrived))
	if err != nil || !inserted {
		return entered{}, err
	}
	// The Enrollment begins (automation.enrolled) and, made already ended, ends in the same
	// transaction: the Contact entered and left.
	begun, err := s.AutomationRun().Query().Where(automationrun.ID(runID)).WithContact().Only(ctx)
	if err != nil {
		return entered{}, err
	}
	if err := publishEnrollment(ctx, s, pub, systemevent.AutomationEnrolled, begun); err != nil {
		return entered{}, err
	}
	if ended {
		if err := move(ctx, s, pub, runID, ending, endedAt, nil); err != nil {
			return entered{}, err
		}
		return entered{runID: runID}, nil
	}
	if !stepping {
		// A version of no Steps has nothing to run: the Enrollment completes as it begins.
		if err := completeRun(ctx, s, pub, runID, "", now); err != nil {
			return entered{}, err
		}
		return entered{runID: runID}, nil
	}
	if err := reach(ctx, s, runID, first.ID, now, arrived); err != nil {
		return entered{}, err
	}
	return entered{runID: runID, stepping: true}, nil
}

// lockContact locks the Contact's row to the end of the transaction s is bound to (contactlock):
// the one point that orders the making of the Contact's Enrollments against the exit and goal
// Events that end them, and against a tag step's change of its Tags. It comes before any write of
// an Enrollment in the transaction (the lock order above). false when the Contact is not in the
// Workspace.
func (m *StepExecution) lockContact(ctx context.Context, s *ent.Scoped, contactID int64) (contactlock.Locked, bool, error) {
	held, err := m.locks.Lock(ctx, s, contactID)
	if ent.IsNotFound(err) {
		return contactlock.Locked{}, false, nil
	}
	return held, err == nil, err
}

// liveShared stands for an Automation share-locked to the end of the transaction with its live
// version read under the lock (shareLive, ADR 0108): a Publish, which locks the row exclusively,
// either finished before the version was read or waits for the Enrollment begun on it. Only
// shareLive makes one, and begin takes it.
type liveShared struct {
	row *ent.Automation
}

// shareLive share-locks the Automation row, archived or not, and reads its live version under the
// lock, so the Enrollment never begins on a version that is no longer live. ErrNoVersion when the
// Automation has none; an ent not-found error when it is not in the Workspace.
func shareLive(ctx context.Context, s *ent.Scoped, automationID int64) (liveShared, error) {
	a, err := s.Automation().QueryWithArchived().Where(automation.ID(automationID)).WithLiveVersion().ForShare().Only(ctx)
	if err != nil {
		return liveShared{}, err
	}
	// The Enrollment begins on the Automation's live version and stays on it (ADR 0049, ADR 0065).
	if a.LiveVersionID == nil {
		return liveShared{}, ErrNoVersion
	}
	return liveShared{row: a}, nil
}

// begin writes the Enrollment of the Contact held in the Automation a, on a's live version, both
// locked to the end of the transaction (ADR 0099, ADR 0108), numbered ordinal among the Contact's
// Enrollments in the Automation, standing on that version's first Step (none for a version of no
// Steps) and parked there until due when that Step is a wait, keeping the chain of o with a's own
// Automation (ADR 0127). One cause enrolls a Contact in an Automation once (cause.insertRun): an
// Enrollment it made already is the answer, and nothing is written (inserted false).
func begin(ctx context.Context, s *ent.Scoped, held contactlock.Locked, a liveShared, o origin, ordinal int, due *time.Time) (runID int64, inserted bool, err error) {
	create := s.AutomationRun().Create().
		SetAutomationID(a.row.ID).
		SetVersionID(*a.row.LiveVersionID).
		SetContactID(held.Contact().ID).
		SetOrdinal(ordinal).
		SetBeganAt(o.OccurredAt).
		SetChain(events.AutomationSet(o.Chain, a.row.ID)).
		SetNillableKeyValue(o.KeyValue).
		SetNillableResumeAt(due)
	if first, ok := automationstep.First(a.row.Edges.LiveVersion.Steps); ok {
		create.SetCurrentStepID(first.ID)
	}
	if o.Action != "" {
		create.SetEventAction(o.Action)
	}
	if len(o.Properties) > 0 {
		create.SetEventProperties(o.Properties)
	}
	runID, err = o.Cause.insertRun(ctx, create)
	inserted, err = db.Inserted(err)
	return runID, inserted, err
}

// endedSince is the end the first exit or goal Event of the version recorded for the Contact since
// the Event of o occurred makes, and when that Event occurred, if there is one (ADR 0133): an Event
// of an exit's or the goal's action whose properties pass its filter, run in Postgres. The Event
// the Enrollment begins on (o.BegunOnSourceID: its Trigger's, or the one its Rule fired on) never
// ends it, and a keyed Enrollment ends only on an
// Event carrying none of the key or o's value (ofKey, ADR 0165). Like the trigger consumer it
// leaves out loaded history and a machine open or click (ADR 0053); an exit and the goal at the
// same moment are taken in the order they were recorded, and the Event found ends the Enrollment as
// endingOf says.
func endedSince(ctx context.Context, s *ent.Scoped, v *ent.AutomationVersion, contactID int64, o origin) (trigger, time.Time, bool, error) {
	exits, goal := ExitsOf(v), v.Goal
	matchers := slices.Clone(exits)
	if goal != nil {
		matchers = append(matchers, *goal)
	}
	if len(matchers) == 0 {
		return "", time.Time{}, false, nil
	}
	ending := make([]predicate.Event, 0, len(matchers))
	for _, m := range matchers {
		p, err := byHuman(m)
		if err != nil {
			return "", time.Time{}, false, err
		}
		ending = append(ending, p)
	}
	where := []predicate.Event{
		event.ContactID(contactID),
		event.Or(ending...),
		event.OccurredAtGTE(o.OccurredAt),
		event.Backfill(false),
	}
	if o.BegunOnSourceID != "" {
		where = append(where, event.Or(event.SourceIDIsNil(), event.SourceIDNEQ(o.BegunOnSourceID)))
	}
	if key := v.EnrollmentKey; key != nil && o.KeyValue != nil {
		where = append(where, ofKey(*key, *o.KeyValue))
	}
	first, err := s.Event().Query().
		Where(where...).
		Order(ent.Asc(event.FieldOccurredAt), ent.Asc(event.FieldID)).
		First(ctx)
	if ent.IsNotFound(err) {
		return "", time.Time{}, false, nil
	}
	if err != nil {
		return "", time.Time{}, false, err
	}
	t, ends, err := endingOf(ctx, s, v, first.Action, event.ID(first.ID))
	if err != nil || !ends {
		return "", time.Time{}, false, err
	}
	return t, first.OccurredAt, true, nil
}

// byHuman matches the Events the matcher matches (matching), leaving out an open or a click a
// machine made (ADR 0053).
func byHuman(m eventmatcher.Matcher) (predicate.Event, error) {
	p, err := matching(m)
	if err != nil || !events.IsEngagement(m.Action) {
		return p, err
	}
	return event.And(p, events.HumanOnly()), nil
}

// Exit ends the Contact's active Enrollments in the Automation at the Instance clock's now (status
// exited, no pending resume), and records each end as its System event in the bus transaction s is
// bound to (events.ErrNotInBusTx for a scope bound to none). It reports whether an active
// Enrollment was ended; the finished ones are left alone, and so is one another writer moved first.
// An Enrollment that begins after it is not exited (ADR 0081, ADR 0127).
func (m *Module) Exit(ctx context.Context, s *ent.Scoped, automationID, contactID int64) (bool, error) {
	runIDs, err := s.AutomationRun().Query().
		Where(
			automationrun.AutomationID(automationID),
			automationrun.ContactID(contactID),
			automationrun.StatusEQ(automationrun.StatusActive),
		).
		IDs(ctx)
	if err != nil {
		return false, err
	}
	exited := false
	for _, runID := range runIDs {
		err := exitRun(ctx, m.bus, s, runID, m.now())
		if errors.Is(err, lifecycle.ErrInvalidTransition) {
			continue // another writer moved it out of active first
		}
		if err != nil {
			return false, err
		}
		exited = true
	}
	return exited, nil
}

// exitRun ends one Enrollment by run id at now, the caller's clock, and publishes its end with bus
// in the transaction s is bound to (Bus.PublishIn). lifecycle.ErrInvalidTransition when it is no
// longer active.
func exitRun(ctx context.Context, bus *events.Bus, s *ent.Scoped, runID int64, now time.Time) error {
	end, err := moved(ctx, s, runID, triggerExit, now, nil)
	if err != nil || end == nil {
		return err
	}
	return bus.PublishIn(end.in(ctx), s, end.change)
}

// endOnEvent ends the Contact's active Enrollments that the Event ends (#751, ADR 0133, endingOf):
// one whose version's goal matches it reaches its goal (`goal_reached`), one whose version has an
// exit matching it leaves early (`exited`). Each Enrollment reads the version it stands on, whatever
// the Automation's status or live version now. Only an Event that occurred when the Enrollment began
// (the triggering Event's occurred_at) or later ends it, so loaded history does not end a fresh
// Enrollment, and an Event that came after the trigger but was handled before the Enrollment was
// made still does; the Event that began an Enrollment never ends it. An Enrollment of a keyed
// version ends only on an Event carrying none of the key or its own value (endsKey, ADR 0165), so a
// cancel of order A ends A's series and leaves B's, and a cancel naming no order ends both.
// It matches by Contact alone, and an Enrollment another writer moved first is left alone; the
// Contact's ended Enrollments, however many, are not read. It locks
// the Contact first, as making an Enrollment does, so s is bound to a transaction: an Enrollment
// being made meanwhile is either committed before the active ones are read, or made after this
// transaction and finds the Event among the stored ones.
func (m *StepExecution) endOnEvent(ctx context.Context, s *ent.Scoped, pub events.Publisher, contactID int64, ev Event) error {
	_, locked, err := m.lockContact(ctx, s, contactID)
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
	values := map[string]*string{}
	for _, run := range runs {
		t, ends, err := m.endingFor(ctx, s, run, ev, values)
		if err != nil {
			return fmt.Errorf("end enrollment %d on event: %w", run.ID, err)
		}
		if !ends {
			continue
		}
		// It ends when the Event that ends it occurred, on the clock its beginning was read on.
		err = move(ctx, s, pub, run.ID, t, ev.OccurredAt, nil)
		if errors.Is(err, lifecycle.ErrInvalidTransition) {
			continue // another writer moved it out of active first
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// endingFor is the move the Event makes on the active Enrollment run, whose version is loaded:
// none when the Event occurred before the Enrollment began or is the one it began on (begunBy),
// else its version's ending (endingOf), on a keyed Enrollment only when the Event may act on its
// key value (endsKey). values caches the Event's key values across the Contact's Enrollments.
func (m *StepExecution) endingFor(ctx context.Context, s *ent.Scoped, run *ent.AutomationRun, ev Event, values map[string]*string) (trigger, bool, error) {
	if ev.OccurredAt.Before(run.BeganAt) {
		return "", false, nil
	}
	t, ends, err := endingOf(ctx, s, run.Edges.Version, ev.Action, event.SourceID(ev.SourceID))
	if err != nil || !ends {
		return "", false, err
	}
	own, err := begunBy(ctx, s, m.firings, run, ev.SourceID)
	if err != nil || own {
		return "", false, err
	}
	ends, err = endsKey(ctx, s, run, ev.SourceID, values)
	if err != nil || !ends {
		return "", false, err
	}
	return t, true, nil
}

// endingOf is the move a stored Event of the action, the one row names, makes on an Enrollment
// standing on the version: its goal when the goal matches it (matchesEvent), else exited when one of
// its exits does, else none. The goal comes first, so an Event matching the goal and an exit reaches
// the goal (ADR 0133): the one place that order is stated, for an Event handled and for one an
// Enrollment finds recorded as it is made (endedSince).
func endingOf(ctx context.Context, s *ent.Scoped, v *ent.AutomationVersion, action string, row predicate.Event) (trigger, bool, error) {
	if v.Goal != nil {
		reached, err := matchesEvent(ctx, s, *v.Goal, action, row)
		if err != nil || reached {
			return triggerReachGoal, reached, err
		}
	}
	for _, exit := range ExitsOf(v) {
		exited, err := matchesEvent(ctx, s, exit, action, row)
		if err != nil || exited {
			return triggerExit, exited, err
		}
	}
	return "", false, nil
}

// triggeredBy lists the active Automations whose live version's Trigger the Event matches (ADR
// 0065, ADR 0133): the candidates by the action, the hot lookup the action's column serves, then
// each candidate's filter run on the stored Event (matchesEvent).
func triggeredBy(ctx context.Context, s *ent.Scoped, ev Event) ([]int64, error) {
	candidates, err := s.Automation().Query().
		Where(
			automation.StatusEQ(automation.StatusActive),
			automation.HasLiveVersionWith(automationversion.TriggerAction(ev.Action)),
		).
		WithLiveVersion().
		All(ctx)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for _, a := range candidates {
		ok, err := matchesEvent(ctx, s, eventMatcherOf(a.Edges.LiveVersion), ev.Action, event.SourceID(ev.SourceID))
		if err != nil {
			return nil, err
		}
		if ok {
			ids = append(ids, a.ID)
		}
	}
	return ids, nil
}
