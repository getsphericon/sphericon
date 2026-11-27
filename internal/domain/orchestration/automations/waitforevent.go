package automations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationsteprecord"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// A Wait for event (ADR 0124) holds an Enrollment until an Event it awaits arrives or its timeout
// passes. Reaching it records its deadline as a wait's due moment (arrive), so its job runs at the
// deadline (timeout); an Event recorded meanwhile wakes the Enrollment at once (wake). Both move it
// off the Step by the compare-and-set on its cursor (advanceRun), so the first to commit wins and
// the other changes nothing.

// validateWaitForEvent checks a Wait for event on its own (ADR 0124, ADR 0136): its timeout is
// greater than zero, as a wait's delay is whole seconds; its name has its form (a bound of the
// contract, in the kernel's form); and it awaits at least one Event matcher, each one a matcher an
// Automation may start or end on (matcherProblem), and none twice. What the matchers' filters and
// its Condition name in the Workspace is checkWaits'. Whether its name is its own across the
// Automation is checkWaitNames' and checkWaitNamesHeld's, and whether an exit or goal ends the
// Enrollment on what it awaits is checkAwaitedEndings'.
func validateWaitForEvent(st *automationstep.Step, w automationstep.WaitForEvent) error {
	if w.TimeoutSeconds <= 0 {
		return refuse(st, "a wait for event needs a timeout greater than zero")
	}
	for _, err := range []error{
		validation.MaxLength(fieldname.Name, w.Name, bounds.WaitForEventNameMaxLength),
		validation.Pattern(fieldname.Name, w.Name, bounds.WaitForEventNamePattern),
	} {
		if bound, ok := boundRefusal(err); ok {
			return &stepRefusalError{step: st.ID, bound: bound, boundAt: bound.Field()}
		}
	}
	switch a := w.Awaits.(type) {
	case automationstep.AwaitedEvent:
		err := validation.MinItems(fieldname.Matchers, len(a.Matchers), bounds.AwaitedMatchersMinItems)
		if bound, ok := boundRefusal(err); ok {
			return &stepRefusalError{step: st.ID, bound: bound, boundAt: fieldname.Awaits + "." + bound.Field()}
		}
		var seen []eventmatcher.Matcher
		for _, matcher := range a.Matchers {
			if problem := matcherProblem(matcher); problem != "" {
				return refuse(st, "an awaited event: "+problem)
			}
			same, err := sameMatcherIn(seen, matcher)
			if err != nil {
				return err
			}
			if same {
				return refuse(st, fmt.Sprintf("it awaits %q with the same filter twice", strings.TrimSpace(matcher.Action)))
			}
			seen = append(seen, matcher)
		}
	case nil:
		return refuse(st, "a wait for event needs what it waits for")
	}
	return nil
}

// sameMatcherIn reports whether one of seen is m: the same action, trimmed, and the same filter or
// none. Two matchers of one action with different filters are two things to wait for.
func sameMatcherIn(seen []eventmatcher.Matcher, m eventmatcher.Matcher) (bool, error) {
	filter, err := json.Marshal(m.Filter)
	if err != nil {
		return false, err
	}
	for _, other := range seen {
		if strings.TrimSpace(other.Action) != strings.TrimSpace(m.Action) {
			continue
		}
		otherFilter, err := json.Marshal(other.Filter)
		if err != nil {
			return false, err
		}
		if bytes.Equal(otherFilter, filter) {
			return true, nil
		}
	}
	return false, nil
}

// checkWaits checks what each Wait for event of the tree awaits against the Workspace: a matcher's
// filter names only the Tags, Custom fields, Broadcasts and Automations it has (checkNamed), and
// its Condition is checked as a Branch path's is, bounded as a Rule's (condition.Module
// .CheckWaitForEvent). A refusal names the Step; one of a Broadcast or Automation the Workspace
// does not have is ErrUnknownSender (ADR 0153).
func (m *Module) checkWaits(ctx context.Context, s *ent.Scoped, steps automationstep.Path) error {
	for st, w := range automationstep.WaitsForEvents(steps) {
		a, ok := w.Awaits.(automationstep.AwaitedEvent)
		if !ok {
			continue
		}
		for _, matcher := range a.Matchers {
			refusal := func(why string) error { return refuse(st, "an awaited event: "+why) }
			if err := m.checkNamed(ctx, s, matcher, refusal); err != nil {
				return err
			}
		}
		if a.Condition == nil {
			continue
		}
		if size := a.Condition.Size(); size > rules.MaxConditionNodes {
			return refuse(st, fmt.Sprintf("a condition has at most %d groups and tests, this one has %d", rules.MaxConditionNodes, size))
		}
		err := m.conditions.CheckWaitForEvent(ctx, s, *a.Condition)
		if errors.Is(err, condition.ErrStorage) {
			return err
		}
		if err != nil {
			return refuse(st, "the condition: "+err.Error())
		}
		unknown, err := m.senders.Unknown(ctx, s, condition.NamedSenders(a.Condition.Group))
		if err != nil {
			return err
		}
		if unknown != nil {
			return ErrUnknownSender.With(fmt.Sprintf("step %s (%s): the condition: %s", st.ID, st.Kind(), unknown.Missing()))
		}
	}
	return nil
}

// checkWaitNames refuses, naming the second, two Wait for event Steps of the tree with one name
// (ADR 0136): a name means one Step.
func checkWaitNames(steps automationstep.Path) error {
	named := map[string]automationstep.ID{}
	for st, w := range automationstep.WaitsForEvents(steps) {
		if held, ok := named[w.Name]; ok {
			return refuse(st, fmt.Sprintf("the name %q is step %s's", w.Name, held))
		}
		named[w.Name] = st.ID
	}
	return nil
}

// checkWaitNamesHeld refuses, naming the Step, a Wait for event that the steps rename, or that
// takes a name another Step id holds, in any version of the Automation automationID, the draft
// being written over and a Wait for event removed since included (ADR 0136): a name belongs to one
// Step id for the Automation's whole life, so `awaited.<name>` never means two Steps. Called under
// the Workspace lock, so a concurrent save of the Automation cannot slip a version past it.
func checkWaitNamesHeld(ctx context.Context, s *ent.Scoped, automationID int64, steps automationstep.Path) error {
	versions, err := s.AutomationVersion().Query().
		Where(automationversion.AutomationID(automationID)).
		Select(automationversion.FieldSteps).
		All(ctx)
	if err != nil {
		return err
	}
	nameOf := map[automationstep.ID]string{}
	holderOf := map[string]automationstep.ID{}
	for _, v := range versions {
		for st, w := range automationstep.WaitsForEvents(v.Steps) {
			nameOf[st.ID], holderOf[w.Name] = w.Name, st.ID
		}
	}
	for st, w := range automationstep.WaitsForEvents(steps) {
		if was, ok := nameOf[st.ID]; ok && was != w.Name {
			return refuse(st, fmt.Sprintf("its name is %q, fixed when it was created", was))
		}
		if holder, ok := holderOf[w.Name]; ok && holder != st.ID {
			return refuse(st, fmt.Sprintf("the name %q is step %s's, in a version of the automation", w.Name, holder))
		}
	}
	return nil
}

// checkAwaitedEndings refuses, naming the Step, a Wait for event that awaits an action an exit or
// the goal of the definition matches with no filter (ADR 0124 as amended by ADR 0133): every such
// Event ends the Enrollment first, so `arrived` could never be taken. One with a filter is allowed,
// since an Event its filter misses still arrives.
func checkAwaitedEndings(def definition) error {
	ending := slices.Clone(def.exits)
	if def.goal != nil {
		ending = append(ending, *def.goal)
	}
	unfiltered := map[string]bool{}
	for _, m := range ending {
		if m.Filter == nil {
			unfiltered[m.Action] = true
		}
	}
	for st, w := range automationstep.WaitsForEvents(def.steps) {
		for _, action := range awaitedActions(w) {
			if unfiltered[action] {
				return refuse(st, fmt.Sprintf("it awaits %q, which an exit or the goal ends the enrollment on with no filter, so it would never arrive", action))
			}
		}
	}
	return nil
}

// awaitedActions are the Event actions the Wait for event's matchers name.
func awaitedActions(w automationstep.WaitForEvent) []string {
	switch a := w.Awaits.(type) {
	case automationstep.AwaitedEvent:
		return a.Actions()
	case nil:
	}
	return nil
}

// awaitedMatchers are the Event matchers the Wait for event waits for.
func awaitedMatchers(w automationstep.WaitForEvent) []eventmatcher.Matcher {
	switch a := w.Awaits.(type) {
	case automationstep.AwaitedEvent:
		return a.Matchers
	case nil:
	}
	return nil
}

// awaitedCondition is the Wait for event's Condition, nil for none.
func awaitedCondition(w automationstep.WaitForEvent) *conditiontree.WaitForEvent {
	switch a := w.Awaits.(type) {
	case automationstep.AwaitedEvent:
		return a.Condition
	case nil:
	}
	return nil
}

// anyMatcher is the predicate that a stored Event matches one of the matchers (matching): its
// action and its properties passing the matcher's filter, which Postgres runs (ADR 0180).
func anyMatcher(matchers []eventmatcher.Matcher) (predicate.Event, error) {
	preds := make([]predicate.Event, len(matchers))
	for i, m := range matchers {
		p, err := matching(m)
		if err != nil {
			return nil, err
		}
		preds[i] = p
	}
	return event.Or(preds...), nil
}

// conditionHolds reports whether the Wait for event's Condition, if any, holds for the
// Enrollment's Contact now, its property tests reading the Event the Enrollment began with: false
// and unknown are no match (ADR 0124). A failure to read is condition.ErrStorage; any other error
// is a Condition that no longer compiles (a Custom field it tests was retyped since), which fails
// the Enrollment, as a Branch's does.
func (m *StepExecution) conditionHolds(ctx context.Context, s *ent.Scoped, run *ent.AutomationRun, w automationstep.WaitForEvent) (bool, error) {
	cond := awaitedCondition(w)
	if cond == nil {
		return true, nil
	}
	return m.conditions.Holds(ctx, s, condition.WaitForEventInput{ContactID: run.ContactID, Condition: *cond, Properties: run.EventProperties})
}

// awaitWindow is what an Event must be to satisfy a Wait for event an Enrollment stands on: the
// Contact's, matched by one of the awaited matchers (matched), occurred from when the Enrollment
// reached the Step through its deadline, and not the Event the Enrollment began on (begunOn, "" for
// none).
type awaitWindow struct {
	contactID         int64
	matched           predicate.Event
	reached, deadline time.Time
	begunOn           string
}

// satisfying is the one predicate an Event stored for the Contact satisfies the window by (ADR
// 0124), read by the wake on the Event just recorded and by the timeout on every Event recorded:
// one of the awaited actions, occurred within the window, made by a person (a machine open or click
// never counts, ADR 0053), live (loaded history never counts) and not the Event that began the
// Enrollment.
func satisfying(w awaitWindow) []predicate.Event {
	where := []predicate.Event{
		event.ContactID(w.contactID),
		w.matched,
		event.OccurredAtGTE(w.reached),
		event.OccurredAtLTE(w.deadline),
		event.Backfill(false),
		events.HumanOnly(),
	}
	if w.begunOn != "" {
		where = append(where, event.Or(event.SourceIDIsNil(), event.SourceIDNEQ(w.begunOn)))
	}
	return where
}

// windowOf is the window of the Wait for event w that the Enrollment run, standing on it, waits in:
// from its Step record's reached moment through its due moment, the deadline recorded when it
// reached the Step. False when the Enrollment has no open record of the Step (it left it).
func (m *StepExecution) windowOf(ctx context.Context, s *ent.Scoped, run *ent.AutomationRun, step automationstep.ID, w automationstep.WaitForEvent) (awaitWindow, bool, error) {
	rec, err := s.AutomationStepRecord().Query().
		Where(automationsteprecord.RunID(run.ID), automationsteprecord.StepID(step), automationsteprecord.LeftAtIsNil()).
		Only(ctx)
	if ent.IsNotFound(err) {
		return awaitWindow{}, false, nil
	}
	if err != nil {
		return awaitWindow{}, false, err
	}
	if rec.DueAt == nil {
		return awaitWindow{}, false, fmt.Errorf("wait for event step %s of run %d has no deadline", step, run.ID)
	}
	begunOn, err := begunOnSource(ctx, s, m, run)
	if err != nil {
		return awaitWindow{}, false, err
	}
	matched, err := anyMatcher(awaitedMatchers(w))
	if err != nil {
		return awaitWindow{}, false, err
	}
	return awaitWindow{contactID: run.ContactID, matched: matched, reached: rec.ReachedAt, deadline: *rec.DueAt, begunOn: begunOn}, true, nil
}

// begunOnSource is the source id of the Event the Enrollment began on: its Trigger's, or the one
// the Rule whose Firing enrolled it fired on; "" for none.
func begunOnSource(ctx context.Context, s *ent.Scoped, m *StepExecution, run *ent.AutomationRun) (string, error) {
	if run.TriggerSourceID != nil {
		return *run.TriggerSourceID, nil
	}
	if run.FiringID == nil {
		return "", nil
	}
	return m.firings.SourceOf(ctx, s, *run.FiringID)
}

// timeout runs the job of a Wait for event at its deadline (ADR 0124): before it, a job run early
// (a restart) queues itself for the deadline and moves nothing; at it, the Enrollment takes
// `arrived` when a satisfying Event is already recorded, one that occurred within the window but
// was recorded late included, and `timed_out` otherwise. The move is the compare-and-set on the
// cursor, so a wake that committed first leaves this job nothing to move.
func (m *StepExecution) timeout(ctx context.Context, s *ent.Scoped, run *ent.AutomationRun, at place, w automationstep.WaitForEvent) error {
	if run.ResumeAt != nil && run.ResumeAt.After(m.now()) {
		return m.requeue(ctx, s, run.ID, at.step, *run.ResumeAt)
	}
	win, open, err := m.windowOf(ctx, s, run, at.step, w)
	if err != nil || !open {
		return err
	}
	arrived, err := s.Event().Query().Where(satisfying(win)...).Exist(ctx)
	if err != nil {
		return err
	}
	if arrived {
		arrived, err = m.conditionHolds(ctx, s, run, w)
		if errors.Is(err, condition.ErrStorage) {
			return err
		}
		if err != nil {
			return m.fail(ctx, s, run.ID, fmt.Errorf("wait for event step %s: %w", at.step, err))
		}
	}
	path := w.TimedOut.ID
	if arrived {
		path = w.Arrived.ID
	}
	return m.advance(ctx, s, run.ID, at, stepMove{path: &pathTaken{id: path}})
}

// wake moves down `arrived` each of the Contact's active Enrollments that stands on a Wait for
// event the Event satisfies (satisfying, on the Event stored with ev.SourceID), each in a move of
// its own. An Enrollment that left the Step, or that the timeout moved first, is left as it is.
func (m *StepExecution) wake(ctx context.Context, s *ent.Scoped, contactID int64, ev Event) error {
	if ev.SourceID == "" {
		return nil // an Event with no stored row satisfies no wait
	}
	runs, err := s.AutomationRun().Query().
		Where(automationrun.ContactID(contactID), automationrun.StatusEQ(automationrun.StatusActive), automationrun.CurrentStepIDNotNil()).
		WithVersion().
		All(ctx)
	if err != nil {
		return err
	}
	for _, run := range runs {
		index := automationstep.NewIndex(run.Edges.Version.Steps)
		st, ok := index.Step(*run.CurrentStepID)
		if !ok {
			continue
		}
		w, waits := st.Payload.(automationstep.WaitForEvent)
		if !waits || !slices.Contains(awaitedActions(w), ev.Action) {
			continue
		}
		win, open, err := m.windowOf(ctx, s, run, st.ID, w)
		if err != nil {
			return fmt.Errorf("wake enrollment %d: %w", run.ID, err)
		}
		if !open {
			continue
		}
		satisfied, err := s.Event().Query().Where(append(satisfying(win), event.SourceID(ev.SourceID))...).Exist(ctx)
		if err != nil {
			return fmt.Errorf("wake enrollment %d: %w", run.ID, err)
		}
		if !satisfied {
			continue
		}
		holds, err := m.conditionHolds(ctx, s, run, w)
		if errors.Is(err, condition.ErrStorage) {
			return fmt.Errorf("wake enrollment %d: %w", run.ID, err)
		}
		if err != nil {
			// The Enrollment fails, as at a Branch; the Event goes on to wake and enroll the others.
			if err := quiet(m.failIn(ctx, s, run.ID, failure{})); err != nil {
				return err
			}
			continue
		}
		if !holds {
			continue
		}
		at := place{index: index, step: st.ID}
		if err := m.advance(ctx, s, run.ID, at, stepMove{path: &pathTaken{id: w.Arrived.ID}}); err != nil {
			return err
		}
	}
	return nil
}
