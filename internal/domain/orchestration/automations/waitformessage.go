package automations

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationsteprecord"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// A Wait for event on an open or a click (ADR 0124) waits for the first human open, or click, of
// the message the send Step it names sent to the Enrollment, with the meaning of a Branch's test of
// that message (ADR 0122). Its job runs as soon as the Enrollment reaches it (queuedAt), which takes
// what it already knows, and again at the deadline; an open or a click recorded meanwhile wakes the
// Enrollment (wakeOnMessage). Both move it off the Step by the compare-and-set on its cursor
// (advanceRun), so the first to commit wins. When the app cannot know whether the message was opened
// or clicked, the Step takes its unknown path, never `timed_out`: people who refused tracking never
// get the "you did not open it" resend. It keeps no copy of what arrived.

// heard is what a Wait for event on an open or a click reads of the message the send Step it names
// sent to the Enrollment: whether it went out tracked (false when there is none: the Step was
// skipped, or the Enrollment never passed it), when its tracking was withdrawn since (ADR 0120),
// and whether a human open, or click, of it is recorded, whenever it happened.
type heard struct {
	sentTracked bool
	withdrawnAt *time.Time
	engaged     bool
}

// engagementActions are the Events of an open or a click of a message: an email's open, or a
// click on any Channel that tracks them.
func engagementActions(e conditiontree.Engagement) []string {
	switch e {
	case conditiontree.MessageOpened:
		return []string{systemevent.EmailOpened}
	case conditiontree.MessageClicked:
		return events.ClickActions
	}
	return nil
}

// stamped reports whether the open or click e of the message is stamped on it.
func stamped(msg *ent.OutboundMessage, e conditiontree.Engagement) bool {
	switch e {
	case conditiontree.MessageOpened:
		return msg.OpenedAt != nil
	case conditiontree.MessageClicked:
		return msg.ClickedAt != nil
	}
	return false
}

// heardOf reads the message the send Step a names sent to the Enrollment run. It is engaged when
// the open or click is stamped on it or recorded as an Event of a person: the stamp is written by a
// job of its own that may not have run yet, so the recorded Event is read too, and a machine's open
// or click, or loaded history, never counts (ADR 0053).
func heardOf(ctx context.Context, s *ent.Scoped, run *ent.AutomationRun, a automationstep.AwaitedMessage) (heard, error) {
	msg, err := s.OutboundMessage().Query().
		Where(outboundmessage.AutomationRunID(run.ID), outboundmessage.AutomationStepID(a.Step)).
		First(ctx)
	if ent.IsNotFound(err) {
		return heard{}, nil
	}
	if err != nil {
		return heard{}, fmt.Errorf("read the message the wait for event awaits: %w", err)
	}
	h := heard{sentTracked: msg.Status == outboundmessage.StatusSent && msg.Tracked, withdrawnAt: msg.TrackingWithdrawnAt}
	if !h.sentTracked {
		return h, nil
	}
	if stamped(msg, a.Engagement) {
		h.engaged = true
		return h, nil
	}
	h.engaged, err = s.Event().Query().
		Where(
			event.ContactID(run.ContactID),
			event.ActionIn(engagementActions(a.Engagement)...),
			events.NamesMessageIn([]int64{msg.ID}),
			events.HumanOnly(),
			event.Backfill(false),
		).
		Exist(ctx)
	if err != nil {
		return heard{}, fmt.Errorf("read the engagement the wait for event awaits: %w", err)
	}
	return h, nil
}

// messagePath is the path a Wait for event on an open or a click takes for an Enrollment that
// reached it at reached, by what it heard of the message, before its deadline or at it; false while
// it waits on. The unknown path is taken at once when the message went out untracked, there is
// none, or its tracking was withdrawn before the Enrollment reached the Step; arrived when it was
// opened or clicked and is still tracked. At the deadline, arrived when it was opened or clicked,
// else the unknown path when its tracking was withdrawn during the wait, else timed out. Before the
// deadline, a withdrawal during the wait leaves it waiting, so a job run early (a restart) decides
// what the deadline would not.
func messagePath(w automationstep.WaitForEvent, h heard, reached time.Time, atDeadline bool) (automationstep.ID, bool) {
	unknown := unknownPath(w)
	withdrawnDuring := h.withdrawnAt != nil && h.withdrawnAt.After(reached)
	switch {
	case !h.sentTracked, h.withdrawnAt != nil && !withdrawnDuring:
		return unknown, true
	case !atDeadline:
		if h.engaged && !withdrawnDuring {
			return w.Arrived.ID, true
		}
		return "", false
	case h.engaged:
		return w.Arrived.ID, true
	case withdrawnDuring:
		return unknown, true
	default:
		return w.TimedOut.ID, true
	}
}

// unknownPath is the id of the Wait for event's unknown path. Publish refuses one on an open or a
// click without it (checkUnknownPaths), so a version that runs has it; were it missing, timed out
// is taken, as a Branch takes otherwise.
func unknownPath(w automationstep.WaitForEvent) automationstep.ID {
	if w.Unknown == nil {
		return w.TimedOut.ID
	}
	return w.Unknown.ID
}

// awaitMessage runs the job of a Wait for event on an open or a click: queued as the Enrollment
// reaches it, it takes the path messagePath decides by the message as it is now, and, while that is
// none, queues itself again for the deadline its Step record keeps. The move is the compare-and-set
// on the cursor, so a wake that committed first leaves this job nothing to move.
func (m *StepExecution) awaitMessage(ctx context.Context, s *ent.Scoped, run *ent.AutomationRun, at place, w automationstep.WaitForEvent, a automationstep.AwaitedMessage) error {
	rec, err := s.AutomationStepRecord().Query().
		Where(automationsteprecord.RunID(run.ID), automationsteprecord.StepID(at.step), automationsteprecord.LeftAtIsNil()).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if rec.DueAt == nil {
		return fmt.Errorf("wait for event step %s of run %d has no deadline", at.step, run.ID)
	}
	h, err := heardOf(ctx, s, run, a)
	if err != nil {
		return err
	}
	path, decided := messagePath(w, h, rec.ReachedAt, !rec.DueAt.After(m.now()))
	if !decided {
		return m.requeue(ctx, s, run.ID, at.step, *rec.DueAt)
	}
	return m.advance(ctx, s, run.ID, at, stepMove{path: &pathTaken{id: path}})
}

// queuedAt is when the job of the Step next, reached with what it decided on arrival, runs: a
// wait's at its due moment, and a Wait for event on an open or a click's at once, to take what it
// already knows (awaitMessage), its deadline still kept as its due moment.
func queuedAt(next *automationstep.Step, arrived automationstep.Due) *time.Time {
	if w, ok := next.Payload.(automationstep.WaitForEvent); ok {
		if _, onMessage := w.Awaits.(automationstep.AwaitedMessage); onMessage {
			return nil
		}
	}
	return dueAt(arrived)
}

// wakeOnMessage moves down `arrived` the Enrollment standing on a Wait for event that waits for the
// open or click the Event of the Contact is, of the message the Step it names sent that Enrollment,
// while the message still counts as tracked. An Event of a machine, loaded history or any other
// Event wakes nothing, nor does one of a message no wait awaits; an Enrollment that left the Step,
// or that the deadline moved first, is left as it is.
func (m *StepExecution) wakeOnMessage(ctx context.Context, s *ent.Scoped, contactID int64, ev Event) error {
	if ev.SourceID == "" || !events.IsEngagement(ev.Action) {
		return nil
	}
	stored, err := s.Event().Query().
		Where(event.ContactID(contactID), event.SourceID(ev.SourceID), events.HumanOnly(), event.Backfill(false)).
		First(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	messageID, ok := events.ClickedMessageID(stored)
	if !ok {
		messageID, ok = events.OutboundMessageID(stored)
	}
	if !ok {
		return nil
	}
	msg, err := s.OutboundMessage().Query().
		Where(outboundmessage.ID(messageID), outboundmessage.AutomationRunIDNotNil(), outboundmessage.AutomationStepIDNotNil()).
		First(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if msg.Status != outboundmessage.StatusSent || !engagementtracking.IsTracked(msg) {
		return nil
	}
	run, err := s.AutomationRun().Query().
		Where(automationrun.ID(*msg.AutomationRunID), automationrun.ContactID(contactID), automationrun.StatusEQ(automationrun.StatusActive), automationrun.CurrentStepIDNotNil()).
		WithVersion().
		First(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	index := automationstep.NewIndex(run.Edges.Version.Steps)
	st, ok := index.Step(*run.CurrentStepID)
	if !ok {
		return nil
	}
	w, waits := st.Payload.(automationstep.WaitForEvent)
	if !waits {
		return nil
	}
	a, onMessage := w.Awaits.(automationstep.AwaitedMessage)
	if !onMessage || a.Step != *msg.AutomationStepID || !slices.Contains(engagementActions(a.Engagement), stored.Action) {
		return nil
	}
	return m.advance(ctx, s, run.ID, place{index: index, step: st.ID}, stepMove{path: &pathTaken{id: w.Arrived.ID}})
}
