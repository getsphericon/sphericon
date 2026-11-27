package automations

import (
	"context"
	"fmt"
	"slices"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationsteprecord"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// A Wait for event on an open or a click (ADR 0124) waits for the first human open, or click, of
// the message the send Step it names sent to the Enrollment, with the meaning of a Branch's test of
// that message (ADR 0122). The move that reaches it decides, in its transaction, what it already
// knows (reachedPath), and the job queued with it takes that path at once; one that does not know
// yet waits for its deadline, its job queued then (deadlinePath). An open or a click recorded
// meanwhile wakes the Enrollment (wakeOnMessage). Both move it off the Step by the compare-and-set
// on its cursor (advanceRun), so the first to commit wins. When the app cannot know whether the
// message was opened or clicked, the Step takes its unknown path, never `timed_out`: people who
// refused tracking never get the "you did not open it" resend. It keeps no copy of what arrived.

// heard is what a Wait for event on an open or a click reads of the message the send Step it names
// sent to the Enrollment: the message as a Branch reads it (sentMessageOf; nil when there is none:
// the Step was skipped, or the Enrollment never passed it), and whether a human open, or click, of
// it is stamped or recorded, whenever it happened.
type heard struct {
	sent    *condition.SentMessage
	engaged bool
}

// tracked reports whether the message was sent tracked and its tracking was not withdrawn since.
func (h heard) tracked() bool {
	return h.sent != nil && h.sent.Tracked
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

// runHeard is heardOf the message the Enrollment runID was sent, read in the transaction of the move
// that reaches the wait (arrive) with the message's row locked against a hit on it to its end. A
// hit locks the row FOR SHARE while it records its open or click (engagement.Recorder), which a
// lock FOR NO KEY UPDATE conflicts with and FOR SHARE would not: a hit that took the row first
// commits before this read, which sees its Event; one that comes after waits for the move to commit,
// and the wake of its Event then finds the Enrollment on the wait. An open in the seconds between
// the send and the Step is never left to the deadline (ADR 0124).
func runHeard(ctx context.Context, s *ent.Scoped, runID int64, a automationstep.AwaitedMessage) (heard, error) {
	run, err := s.AutomationRun().Get(ctx, runID)
	if err != nil {
		return heard{}, err
	}
	_, err = s.OutboundMessage().Query().
		Where(outboundmessage.AutomationRunID(runID), outboundmessage.AutomationStepID(a.Step)).
		ForUpdate(func(o *sql.LockOptions) { o.Strength = sql.LockNoKeyUpdate }).
		IDs(ctx)
	if err != nil {
		return heard{}, fmt.Errorf("lock the message the wait for event awaits: %w", err)
	}
	return heardOf(ctx, s, run, a)
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
	sent := sentMessageOf(msg)
	h := heard{sent: &sent}
	if !h.tracked() {
		return h, nil // an untracked message's opens and clicks are not to be known
	}
	if sent.Stamped(a.Engagement) {
		h.engaged = true
		return h, nil
	}
	h.engaged, err = s.Event().Query().
		Where(
			event.ContactID(run.ContactID),
			event.ActionIn(engagementActions(a.Engagement)...),
			events.NamesMessageIn([]int64{msg.ID}),
			events.HumanOnly(),
			events.Live(),
		).
		Exist(ctx)
	if err != nil {
		return heard{}, fmt.Errorf("read the engagement the wait for event awaits: %w", err)
	}
	return h, nil
}

// reachedPath is the path a Wait for event on an open or a click takes as the Enrollment reaches it,
// by what it heard of the message then; false while it waits on. The unknown path when the message
// is not tracked (it went out untracked, there is none, or its tracking was withdrawn since), and
// arrived when it was opened or clicked.
func reachedPath(w automationstep.WaitForEvent, h heard) (automationstep.ID, bool) {
	switch {
	case !h.tracked():
		return unknownPath(w), true
	case h.engaged:
		return w.Arrived.ID, true
	default:
		return "", false
	}
}

// deadlinePath is the path a Wait for event on an open or a click that waited takes at its deadline,
// by what it heard of the message then: arrived when it was opened or clicked, else the unknown path
// when its tracking was withdrawn during the wait (it was tracked when the Enrollment reached the
// Step, or the Step would not have waited), else timed out.
func deadlinePath(w automationstep.WaitForEvent, h heard) automationstep.ID {
	switch {
	case h.engaged:
		return w.Arrived.ID
	case !h.tracked():
		return unknownPath(w)
	default:
		return w.TimedOut.ID
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

// awaitMessage runs the job of a Wait for event on an open or a click. Queued at once with the path
// the move that reached it decided, it takes that path, whatever happened since. Queued for the
// deadline its Step record keeps, it takes the path deadlinePath decides by the message as it is
// then; run early (a restart), it queues itself again for the deadline. The move is the
// compare-and-set on the cursor, so a wake that committed first leaves this job nothing to move.
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
	if rec.DecidedPathID != nil {
		return m.advance(ctx, s, run.ID, at, stepMove{path: &pathTaken{id: *rec.DecidedPathID}})
	}
	if rec.DueAt == nil {
		return fmt.Errorf("wait for event step %s of run %d has neither a decided path nor a deadline", at.step, run.ID)
	}
	if rec.DueAt.After(m.now()) {
		return m.requeue(ctx, s, run.ID, at.step, *rec.DueAt)
	}
	h, err := heardOf(ctx, s, run, a)
	if err != nil {
		return err
	}
	return m.advance(ctx, s, run.ID, at, stepMove{path: &pathTaken{id: deadlinePath(w, h)}})
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
		Where(event.ContactID(contactID), event.SourceID(ev.SourceID), events.HumanOnly(), events.Live()).
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
	if !sentMessageOf(msg).Tracked {
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
	a, onMessage := w.Message()
	if !onMessage || a.Step != *msg.AutomationStepID || !slices.Contains(engagementActions(a.Engagement), stored.Action) {
		return nil
	}
	return m.advance(ctx, s, run.ID, place{index: index, step: st.ID}, stepMove{path: &pathTaken{id: w.Arrived.ID}})
}
