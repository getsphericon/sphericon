package automations_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationsteprecord"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// waitTimeout is how long the Wait for event of these tests waits.
const waitTimeout = time.Hour

// waiting is a live Automation whose first Step is a Wait for event that tags `buyer` on arrived
// and `reminded` on timed out, both paths rejoining on an hour's wait: the ids of the Automation, the
// Wait for event and the first Step of each path, and its Contact's Enrollment once one is made.
type waiting struct {
	env                     *testhelper.TestEnv
	clock                   *testhelper.Clock
	automationID            int64
	wait, arrived, timedOut automationstep.ID
}

// waitFor publishes, on an env whose clock the test moves, an Automation triggered on trigger whose
// first Step waits an hour for an Event of the actions, ended by the version's goal (nil for none).
func waitFor(t *testing.T, trigger string, actions []string, goal *eventmatcher.Matcher) waiting {
	t.Helper()
	return waitWith(t, trigger, actions, func(in *automations.CreateInput) { in.GoalEvent = goal })
}

// waitWith is waitFor's Automation, its definition otherwise as with sets it (its entry, exits and
// goal).
func waitWith(t *testing.T, trigger string, actions []string, with func(in *automations.CreateInput)) waiting {
	t.Helper()
	matchers := lo.Map(actions, func(action string, _ int) eventmatcher.Matcher { return eventmatcher.Matcher{Action: action} })
	return awaitingWith(t, trigger, automationstep.AwaitedEvent{Matchers: matchers}, with)
}

// awaiting is waitFor of a Wait for event that awaits what awaited says: its matchers and its
// Condition.
func awaiting(t *testing.T, trigger string, awaited automationstep.AwaitedEvent, goal *eventmatcher.Matcher) waiting {
	t.Helper()
	return awaitingWith(t, trigger, awaited, func(in *automations.CreateInput) { in.GoalEvent = goal })
}

// awaitingWith is waitWith of a Wait for event that awaits what awaited says.
func awaitingWith(t *testing.T, trigger string, awaited automationstep.AwaitedEvent, with func(in *automations.CreateInput)) waiting {
	t.Helper()
	clock := testhelper.NewClock(time.Now().UTC().Truncate(time.Second))
	env := testhelper.Setup(t, testhelper.WithClock(clock.Now))
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	wait := automationstep.WaitForEvent{
		Name: "purchase", TimeoutSeconds: int(waitTimeout / time.Second),
		Awaits:   awaited,
		Arrived:  automationstep.Root(automationstep.New(automationstep.ApplyTag{Tag: "buyer"})),
		TimedOut: automationstep.Root(automationstep.New(automationstep.ApplyTag{Tag: "reminded"})),
	}
	in := automations.CreateInput{
		Name: "Waits for a purchase", Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: trigger}},
		Steps: steps(wait, automationstep.Wait{Seconds: 3600}),
	}
	with(&in)
	a, err := m.Create(ctx, s, in)
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, a.ID, testhelper.AutomationRevisionIn(ctx, t, s, a.ID))
	require.NoError(t, err)
	live, err := m.Get(ctx, s, a.ID)
	require.NoError(t, err)
	version, _ := automations.LiveAndDraft(live)
	stored := version.Steps.Steps[0]
	held := stored.Payload.(automationstep.WaitForEvent)
	return waiting{env: env, clock: clock, automationID: a.ID, wait: stored.ID, arrived: held.Arrived.Steps[0].ID, timedOut: held.TimedOut.Steps[0].ID}
}

// enroll enrolls a free Contact on the Trigger's Event, occurred now, and returns it and its
// Enrollment, standing on the Wait for event.
func (w waiting) enroll(t *testing.T, trigger string) (contact *ent.Contact, runID int64) {
	t.Helper()
	c := enrollable(t, w.env.DB.Scoped(fixtures.AcmeID), 0)
	handStored(t, w.env, c.ID, trigger, collected(c.ID, trigger, w.clock.Now(), nil))
	runs := runsOf(t, w.env, w.automationID, c.ID)
	require.Len(t, runs, 1)
	return c, runs[0].ID
}

// handSendLocked stores ev as a write through a Connection without mcp:send stores it, with the
// send lock's mark (ADR 0132), and hands it to the Automations as its trigger job does.
func handSendLocked(t *testing.T, env *testhelper.TestEnv, contactID int64, action string, ev events.DomainEvent) {
	t.Helper()
	require.NoError(t, env.Publisher().Publish(events.SendLocked(t.Context()), ev))
	stored := env.StoredEvents(t, action)
	require.NotEmpty(t, stored)
	last := stored[len(stored)-1]
	require.True(t, last.SendLocked)
	in := automations.OnEventInput{ContactID: contactID, Event: automations.Event{Action: last.Action, SourceID: *last.SourceID, OccurredAt: last.OccurredAt, SendLocked: true}}
	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(t.Context(), env.DB.Scoped(fixtures.AcmeID), in))
}

// timeout runs the Wait for event's job, as it runs at the clock's moment.
func (w waiting) timeout(t *testing.T, runID int64) {
	t.Helper()
	require.NoError(t, testhelper.Module[*automations.StepExecution](w.env).Step(t.Context(), w.env.DB.Scoped(fixtures.AcmeID), runID, w.wait))
}

// standsOn is the Step the Enrollment stands on.
func (w waiting) standsOn(t *testing.T, runID int64) automationstep.ID {
	t.Helper()
	run, err := w.env.DB.AutomationRun.Get(t.Context(), runID)
	require.NoError(t, err)
	return lo.FromPtr(run.CurrentStepID)
}

// waitRecords are the Enrollment's records of the Wait for event.
func (w waiting) waitRecords(t *testing.T, runID int64) []*ent.AutomationStepRecord {
	t.Helper()
	records, err := testhelper.Module[*automations.Module](w.env).StepRecords(t.Context(), w.env.DB.Scoped(fixtures.AcmeID), w.automationID, runID)
	require.NoError(t, err)
	return lo.Filter(records, func(r *ent.AutomationStepRecord, _ int) bool { return r.StepID == w.wait })
}

// pathTaken is the path the Enrollment's one record of the Wait for event says it took.
func (w waiting) pathTaken(t *testing.T, runID int64) automationstep.ID {
	t.Helper()
	records := w.waitRecords(t, runID)
	require.Len(t, records, 1, "one record of the Wait for event")
	return lo.FromPtr(records[0].PathID)
}

// heldPath is the id of the Wait for event's path the first Step of which is first.
func (w waiting) pathOf(t *testing.T, first automationstep.ID) automationstep.ID {
	t.Helper()
	live, err := testhelper.Module[*automations.Module](w.env).Get(t.Context(), w.env.DB.Scoped(fixtures.AcmeID), w.automationID)
	require.NoError(t, err)
	version, _ := automations.LiveAndDraft(live)
	held := version.Steps.Steps[0].Payload.(automationstep.WaitForEvent)
	return lo.Ternary(held.Arrived.Steps[0].ID == first, held.Arrived.ID, held.TimedOut.ID)
}

// outcome names the path the Enrollment's one record of the Wait for event says it took: "arrived"
// or "timed out".
func (w waiting) outcome(t *testing.T, runID int64) string {
	t.Helper()
	names := map[automationstep.ID]string{w.pathOf(t, w.arrived): "arrived", w.pathOf(t, w.timedOut): "timed out"}
	return names[w.pathTaken(t, runID)]
}

// Reaching a Wait for event records its deadline, the reached moment plus the timeout, on the
// Enrollment and in its Step record, and parks it there.
func TestReachingAWaitForEventRecordsItsDeadline(t *testing.T) {
	w := waitFor(t, "trial.started", []string{"order.created"}, nil)
	reached := w.clock.Now()

	_, runID := w.enroll(t, "trial.started")

	run, err := w.env.DB.AutomationRun.Get(t.Context(), runID)
	require.NoError(t, err)
	records := w.waitRecords(t, runID)
	require.Len(t, records, 1)
	require.NotNil(t, run.ResumeAt)
	assert.Equal(t, w.wait, lo.FromPtr(run.CurrentStepID))
	assert.WithinDuration(t, reached.Add(waitTimeout), *run.ResumeAt, 0)
	assert.Equal(t, run.ResumeAt, records[0].DueAt, "the Step record keeps the deadline")
	assert.Nil(t, records[0].LeftAt)
}

// An awaited Event recorded during the wait takes `arrived` at once, and the deadline later
// changes nothing: the timeout's job finds the Enrollment moved on.
func TestAnAwaitedEventDuringTheWaitTakesArrivedAtOnce(t *testing.T) {
	w := waitFor(t, "trial.started", []string{"order.created", "call.booked"}, nil)
	c, runID := w.enroll(t, "trial.started")

	w.clock.Advance(10 * time.Minute)
	handStored(t, w.env, c.ID, "order.created", collected(c.ID, "order.created", w.clock.Now(), nil))
	onArrival := w.standsOn(t, runID)
	w.clock.Advance(waitTimeout)
	w.timeout(t, runID)

	assert.Equal(t, w.arrived, onArrival, "the arrived path's first Step")
	assert.Equal(t, w.arrived, w.standsOn(t, runID), "the deadline moved nothing")
	assert.Equal(t, w.pathOf(t, w.arrived), w.pathTaken(t, runID))
	records := w.waitRecords(t, runID)
	assert.Equal(t, automationsteprecord.OutcomePath, lo.FromPtr(records[0].Outcome))
	assert.WithinDuration(t, w.clock.Now().Add(-waitTimeout), lo.FromPtr(records[0].LeftAt), 0, "it left when the Event arrived")
}

// With no awaited Event the Enrollment takes `timed_out` at the deadline. A restart that runs the
// job before then neither moves the Enrollment nor computes its deadline again.
func TestWithNoEventTheWaitTimesOutAtTheDeadlineAndNotBefore(t *testing.T) {
	w := waitFor(t, "trial.started", []string{"order.created"}, nil)
	_, runID := w.enroll(t, "trial.started")
	deadline := w.clock.Now().Add(waitTimeout)

	w.clock.Advance(waitTimeout / 2)
	w.timeout(t, runID)
	early, err := w.env.DB.AutomationRun.Get(t.Context(), runID)
	require.NoError(t, err)
	w.clock.Set(deadline)
	w.timeout(t, runID)

	assert.Equal(t, w.wait, lo.FromPtr(early.CurrentStepID), "a restart before the deadline moves nothing")
	assert.WithinDuration(t, deadline, lo.FromPtr(early.ResumeAt), 0, "nor waits twice")
	assert.Equal(t, w.timedOut, w.standsOn(t, runID))
	assert.Equal(t, w.pathOf(t, w.timedOut), w.pathTaken(t, runID))
}

// An Event that occurred before the Enrollment reached the Step, the Enrollment's own triggering
// Event, a machine open, loaded history and an Event written through a Connection without mcp:send
// (ADR 0132) each leave the Enrollment waiting, and it times out.
func TestEventsThatDoNotCountLeaveTheEnrollmentWaiting(t *testing.T) {
	for name, c := range map[string]struct {
		trigger, awaited string
		record           func(t *testing.T, w waiting, contact *ent.Contact)
	}{
		"occurred before it reached the step": {"trial.started", "order.created", func(t *testing.T, w waiting, contact *ent.Contact) {
			t.Helper()
			// It reached the Step a minute ago.
			handStored(t, w.env, contact.ID, "order.created", collected(contact.ID, "order.created", w.clock.Now().Add(-2*time.Minute), nil))
		}},
		"its own triggering event": {"order.created", "order.created", func(t *testing.T, w waiting, contact *ent.Contact) {
			t.Helper()
			handLast(t, w.env, contact.ID, "order.created")
		}},
		"a machine open": {"trial.started", systemevent.EmailOpened, func(t *testing.T, w waiting, contact *ent.Contact) {
			t.Helper()
			handStored(t, w.env, contact.ID, systemevent.EmailOpened, &events.EmailEngagement{
				Action: systemevent.EmailOpened, WorkspaceID: fixtures.AcmeID, ContactID: contact.ID, Email: lo.FromPtr(contact.Email),
				OutboundMessageID: fixtures.OutboundMessageTrackingSampleID, Machine: true,
			})
		}},
		"loaded history": {"trial.started", "order.created", func(t *testing.T, w waiting, contact *ent.Contact) {
			t.Helper()
			history := collected(contact.ID, "order.created", w.clock.Now(), nil)
			history.Backfill = true
			handStored(t, w.env, contact.ID, "order.created", history)
		}},
		"written through a connection without mcp:send": {"trial.started", "order.created", func(t *testing.T, w waiting, contact *ent.Contact) {
			t.Helper()
			handSendLocked(t, w.env, contact.ID, "order.created", collected(contact.ID, "order.created", w.clock.Now(), nil))
		}},
	} {
		t.Run(name, func(t *testing.T) {
			w := waitFor(t, c.trigger, []string{c.awaited}, nil)
			contact, runID := w.enroll(t, c.trigger)

			w.clock.Advance(time.Minute)
			c.record(t, w, contact)
			afterTheEvent := w.standsOn(t, runID)
			w.clock.Advance(waitTimeout)
			w.timeout(t, runID)

			assert.Equal(t, w.wait, afterTheEvent, "still waiting")
			assert.Equal(t, w.pathOf(t, w.timedOut), w.pathTaken(t, runID), "the timeout does not count it either")
		})
	}
}

// An Event that occurred within the window but was recorded after the deadline, before the
// timeout's job ran, is found by that job, which takes `arrived`. One that occurred after the
// deadline is not.
func TestTheTimeoutFindsAnEventThatOccurredInTheWindowButWasRecordedLate(t *testing.T) {
	for name, c := range map[string]struct {
		occurredBeforeDeadline time.Duration
		takes                  func(w waiting) automationstep.ID
	}{
		"within the window":  {time.Minute, func(w waiting) automationstep.ID { return w.arrived }},
		"after the deadline": {-time.Minute, func(w waiting) automationstep.ID { return w.timedOut }},
	} {
		t.Run(name, func(t *testing.T) {
			w := waitFor(t, "trial.started", []string{"order.created"}, nil)
			contact, runID := w.enroll(t, "trial.started")
			deadline := w.clock.Now().Add(waitTimeout)

			w.clock.Set(deadline.Add(5 * time.Minute))
			// Recorded, its trigger job not yet run.
			require.NoError(t, w.env.Publisher().Publish(t.Context(), collected(contact.ID, "order.created", deadline.Add(-c.occurredBeforeDeadline), nil)))
			w.timeout(t, runID)

			assert.Equal(t, c.takes(w), w.standsOn(t, runID))
		})
	}
}

// Arrival and the timeout race, and the first to commit wins: in either order the Enrollment ends
// on one path, with one record of the Wait for event, and the other changes nothing. An Event
// recorded after the Enrollment left changes nothing either.
func TestArrivalAndTheTimeoutMoveTheEnrollmentOnce(t *testing.T) {
	t.Run("arrival first", func(t *testing.T) {
		w := waitFor(t, "trial.started", []string{"order.created"}, nil)
		contact, runID := w.enroll(t, "trial.started")

		w.clock.Advance(waitTimeout)
		handStored(t, w.env, contact.ID, "order.created", collected(contact.ID, "order.created", w.clock.Now(), nil))
		w.timeout(t, runID)

		assert.Equal(t, w.arrived, w.standsOn(t, runID))
		assert.Equal(t, w.pathOf(t, w.arrived), w.pathTaken(t, runID))
	})
	t.Run("timeout first", func(t *testing.T) {
		w := waitFor(t, "trial.started", []string{"order.created"}, nil)
		contact, runID := w.enroll(t, "trial.started")

		w.clock.Advance(waitTimeout)
		w.timeout(t, runID)
		handStored(t, w.env, contact.ID, "order.created", collected(contact.ID, "order.created", w.clock.Now(), nil))
		handLast(t, w.env, contact.ID, "order.created")

		assert.Equal(t, w.timedOut, w.standsOn(t, runID))
		assert.Equal(t, w.pathOf(t, w.timedOut), w.pathTaken(t, runID))
	})
}

// An Event that matches both a filtered goal and the wait ends the Enrollment as goal_reached: the
// declared outcome wins wherever the Contact stands. One its filter misses takes `arrived`.
func TestAnEventMatchingAFilteredGoalAndTheWaitReachesTheGoal(t *testing.T) {
	for name, c := range map[string]struct {
		status string
		ends   automationrun.Status
	}{
		"paid":    {"paid", automationrun.StatusGoalReached},
		"pending": {"pending", automationrun.StatusActive},
	} {
		t.Run(name, func(t *testing.T) {
			goal := &eventmatcher.Matcher{Action: "order.created", Filter: propertyEq(t, "status", "paid")}
			w := waitFor(t, "trial.started", []string{"order.created"}, goal)
			contact, runID := w.enroll(t, "trial.started")

			w.clock.Advance(time.Minute)
			handStored(t, w.env, contact.ID, "order.created", collected(contact.ID, "order.created", w.clock.Now(), map[string]any{"status": c.status}))

			assert.Equal(t, c.ends, statusOf(t, w.env, runID))
		})
	}
}

// With an Enrollment key (ADR 0165), an awaited Event carrying another value wakes nothing and the
// timeout does not count it; one carrying an Enrollment's value wakes that Enrollment only; one
// without the property wakes every Enrollment of the Contact.
func TestAnAwaitedEventWakesTheEnrollmentOfItsKeyOnly(t *testing.T) {
	for name, c := range map[string]struct {
		paid map[string]any
		want map[string]string
	}{
		"another value":   {keyed("C"), map[string]string{"A": "timed out", "B": "timed out"}},
		"order A's value": {keyed("A"), map[string]string{"A": "arrived", "B": "timed out"}},
		"no order":        {map[string]any{"total": 30}, map[string]string{"A": "arrived", "B": "arrived"}},
	} {
		t.Run(name, func(t *testing.T) {
			w := waitWith(t, orderPlaced, []string{orderPaid}, func(in *automations.CreateInput) {
				in.Entry = automations.Entry{Policy: automationversion.EntryPolicyOnce, Key: new("order_id")}
			})
			contact := enrollable(t, w.env.DB.Scoped(fixtures.AcmeID), 0)
			order(t, w.env, contact.ID, orderPlaced, w.clock.Now(), keyed("A"))
			order(t, w.env, contact.ID, orderPlaced, w.clock.Now(), keyed("B"))
			runByKey := lo.SliceToMap(runsOf(t, w.env, w.automationID, contact.ID), func(r *ent.AutomationRun) (string, int64) {
				return lo.FromPtr(r.KeyValue), r.ID
			})
			require.Len(t, runByKey, 2)

			w.clock.Advance(time.Minute)
			order(t, w.env, contact.ID, orderPaid, w.clock.Now(), c.paid)
			w.clock.Advance(waitTimeout)
			w.timeout(t, runByKey["A"])
			w.timeout(t, runByKey["B"])

			assert.Equal(t, c.want, map[string]string{"A": w.outcome(t, runByKey["A"]), "B": w.outcome(t, runByKey["B"])})
		})
	}
}

// A Contact enrolled again waits on their own new Enrollment only: the purchase that moved the
// first Enrollment down `arrived` does not count for the second, which times out, and a purchase
// during the second's wait moves the second alone.
func TestAContactEnrolledAgainWaitsOnTheNewEnrollmentOnly(t *testing.T) {
	for name, c := range map[string]struct {
		inTheSecondWait func(t *testing.T, w waiting, contactID int64)
		want            string
	}{
		"no purchase in the second wait": {func(*testing.T, waiting, int64) {}, "timed out"},
		"a purchase in the second wait": {func(t *testing.T, w waiting, contactID int64) {
			t.Helper()
			handStored(t, w.env, contactID, "order.created", collected(contactID, "order.created", w.clock.Now(), nil))
		}, "arrived"},
	} {
		t.Run(name, func(t *testing.T) {
			w := waitWith(t, "trial.started", []string{"order.created"}, func(in *automations.CreateInput) {
				in.Entry = automations.Entry{Policy: automationversion.EntryPolicyAgain}
				in.ExitEvents = []eventmatcher.Matcher{{Action: "trial.cancelled"}}
			})
			contact, first := w.enroll(t, "trial.started")
			w.clock.Advance(time.Minute)
			handStored(t, w.env, contact.ID, "order.created", collected(contact.ID, "order.created", w.clock.Now(), nil))
			w.clock.Advance(time.Minute)
			handStored(t, w.env, contact.ID, "trial.cancelled", collected(contact.ID, "trial.cancelled", w.clock.Now(), nil))
			// Past the Entry cooldown, an hour from the first Enrollment's beginning.
			w.clock.Advance(time.Hour)
			handStored(t, w.env, contact.ID, "trial.started", collected(contact.ID, "trial.started", w.clock.Now(), nil))
			second, ok := lo.Find(runsOf(t, w.env, w.automationID, contact.ID), func(r *ent.AutomationRun) bool { return r.ID != first })
			require.True(t, ok, "enrolled again")
			firstTook := w.outcome(t, first)

			w.clock.Advance(time.Minute)
			c.inTheSecondWait(t, w, contact.ID)
			w.clock.Advance(waitTimeout)
			w.timeout(t, second.ID)

			assert.Equal(t, c.want, w.outcome(t, second.ID))
			assert.Equal(t, "arrived", firstTook)
			assert.Equal(t, "arrived", w.outcome(t, first), "the first Enrollment's wait is as it was")
			assert.Equal(t, automationrun.StatusExited, statusOf(t, w.env, first))
		})
	}
}

// A save is refused, naming the Step, when a Wait for event's timeout is below the contract's bound
// of one second: the owner checks it, whatever surface wrote it.
func TestAWaitForEventWithoutATimeoutIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	steps := automationstep.Root(automationstep.Step{ID: purchaseID, Payload: automationstep.WaitForEvent{
		Name: "purchase", TimeoutSeconds: 0, Awaits: automationstep.AwaitedEvent{Matchers: []eventmatcher.Matcher{{Action: awaitedAction}}},
		Arrived: automationstep.Root(), TimedOut: automationstep.Root(),
	}})

	err := saveVersion(t, env, steps)

	require.ErrorIs(t, err, automations.ErrInvalidStep)
	require.ErrorContains(t, err, "step "+string(purchaseID)+" (wait_for_event): the timeout is 0 seconds, and a wait for event waits at least 1")
}
