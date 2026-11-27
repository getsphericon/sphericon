package automations_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// orderOver100 is the Event matcher of an order.completed whose total is over 100.
func orderOver100(t *testing.T) eventmatcher.Matcher {
	t.Helper()
	return eventmatcher.Matcher{Action: "order.completed", Filter: &conditiontree.EventFilter{Group: groupOf(t,
		`{"kind":"property","key":"total","test":{"kind":"number","operator":"gt","value":100}}`)}}
}

// groupOf is the group whose entries are the leaves written as JSON, all of which must hold.
func groupOf(t *testing.T, entries ...string) conditiontree.Group {
	t.Helper()
	g := conditiontree.Group{Kind: conditiontree.KindGroup, Combinator: conditiontree.And, Entries: []json.RawMessage{}}
	for _, e := range entries {
		require.True(t, json.Valid([]byte(e)), e)
		g.Entries = append(g.Entries, json.RawMessage(e))
	}
	return g
}

// waitCondition is the Wait for event's Condition of the leaves written as JSON.
func waitCondition(t *testing.T, entries ...string) *conditiontree.WaitForEvent {
	t.Helper()
	return &conditiontree.WaitForEvent{Group: groupOf(t, entries...)}
}

// order is order.completed of the Contact, of the total, occurred now.
func (w waiting) order(t *testing.T, c *ent.Contact, total float64) {
	t.Helper()
	handStored(t, w.env, c.ID, "order.completed", collected(c.ID, "order.completed", w.clock.Now(), map[string]any{"total": total}))
}

// With the matcher order.completed where total is over 100, an order of 50 leaves the Contact
// waiting and a later order of 150 takes `arrived` (ADR 0180).
func TestAWaitForEventArrivesOnlyOnAnEventItsMatchersFilterPasses(t *testing.T) {
	w := awaiting(t, "trial.started", automationstep.AwaitedEvent{Matchers: []eventmatcher.Matcher{orderOver100(t)}}, nil)
	c, runID := w.enroll(t, "trial.started")

	w.clock.Advance(time.Minute)
	w.order(t, c, 50)
	afterSmall := w.standsOn(t, runID)
	w.clock.Advance(time.Minute)
	w.order(t, c, 150)

	assert.Equal(t, w.wait, afterSmall, "a small order leaves the Contact waiting")
	assert.Equal(t, w.arrived, w.standsOn(t, runID))
}

// A Wait for event awaiting two matchers takes `arrived` on an Event of either.
func TestAWaitForEventAwaitingTwoMatchersArrivesOnEither(t *testing.T) {
	for name, record := range map[string]func(t *testing.T, w waiting, c *ent.Contact){
		"a large order": func(t *testing.T, w waiting, c *ent.Contact) {
			t.Helper()
			w.order(t, c, 150)
		},
		"a booked call": func(t *testing.T, w waiting, c *ent.Contact) {
			t.Helper()
			handStored(t, w.env, c.ID, "call.booked", collected(c.ID, "call.booked", w.clock.Now(), nil))
		},
	} {
		t.Run(name, func(t *testing.T) {
			w := awaiting(t, "trial.started", automationstep.AwaitedEvent{
				Matchers: []eventmatcher.Matcher{orderOver100(t), {Action: "call.booked"}},
			}, nil)
			c, runID := w.enroll(t, "trial.started")

			w.clock.Advance(time.Minute)
			record(t, w, c)

			assert.Equal(t, w.arrived, w.standsOn(t, runID))
		})
	}
}

// A Wait for event's Condition is read on the Contact's live data and the Event the Enrollment
// began with when the awaited Event is handled: a matching Event arrives only when it holds.
func TestAWaitForEventArrivesOnlyWhenItsConditionHolds(t *testing.T) {
	for name, c := range map[string]struct {
		condition string
		takes     func(w waiting) automationstep.ID
	}{
		"has a tag it lacks": {`{"kind":"tag","name":"vip","operator":"has"}`, func(w waiting) automationstep.ID { return w.wait }},
		"lacks that tag":     {`{"kind":"tag","name":"vip","operator":"doesNotHave"}`, func(w waiting) automationstep.ID { return w.arrived }},
		"its trigger's plan": {
			`{"kind":"property","of":{"kind":"trigger"},"key":"plan","test":{"kind":"text","operator":"eq","value":"pro"}}`,
			func(w waiting) automationstep.ID { return w.arrived },
		},
		"another plan": {
			`{"kind":"property","of":{"kind":"trigger"},"key":"plan","test":{"kind":"text","operator":"eq","value":"team"}}`,
			func(w waiting) automationstep.ID { return w.wait },
		},
	} {
		t.Run(name, func(t *testing.T) {
			w := awaiting(t, "trial.started", automationstep.AwaitedEvent{
				Matchers: []eventmatcher.Matcher{{Action: "order.completed"}}, Condition: waitCondition(t, c.condition),
			}, nil)
			contact := enrollable(t, w.env.DB.Scoped(fixtures.AcmeID), 0)
			handStored(t, w.env, contact.ID, "trial.started", collected(contact.ID, "trial.started", w.clock.Now(), map[string]any{"plan": "pro"}))
			runID := runOf(t, w.env, w.automationID, contact.ID)

			w.clock.Advance(time.Minute)
			w.order(t, contact, 10)

			assert.Equal(t, c.takes(w), w.standsOn(t, runID))
		})
	}
}

// A Condition whose answer is unknown leaves the Contact waiting (ADR 0124): a count of opens is
// unknown for a Contact whose address refused tracking (ADR 0123), so a Condition that holds for any
// tracked Contact, opened or not, holds for the tracked one and leaves the untracked one waiting.
func TestAWaitForEventsUnknownConditionLeavesTheContactWaiting(t *testing.T) {
	openedOrNot := `{"kind":"group","combinator":"or","entries":[` +
		`{"kind":"event","action":"email.opened","operator":"atLeast","times":1},` +
		`{"kind":"event","action":"email.opened","operator":"atMost","times":0}]}`
	w := awaiting(t, "trial.started", automationstep.AwaitedEvent{
		Matchers: []eventmatcher.Matcher{{Action: "order.completed"}}, Condition: waitCondition(t, openedOrNot),
	}, nil)
	ctx := t.Context()
	tracked, trackedRun := w.enroll(t, "trial.started")
	untracked, untrackedRun := w.enroll(t, "trial.started")
	_, err := testhelper.Module[*engagementtracking.Module](w.env).Record(ctx, w.env.DB.Scoped(fixtures.AcmeID), engagementtracking.RecordInput{
		Channel: channels.Email, Destination: lo.FromPtr(untracked.Email), State: engagementtracking.Denied, Provenance: engagementtracking.Manual,
	})
	require.NoError(t, err)

	w.clock.Advance(time.Minute)
	w.order(t, tracked, 10)
	w.order(t, untracked, 10)

	assert.Equal(t, w.arrived, w.standsOn(t, trackedRun))
	assert.Equal(t, w.wait, w.standsOn(t, untrackedRun), "unknown is no match")
}

// The timeout's lookup applies the same matchers and Condition: it skips a recorded Event its
// matcher's filter fails, and one recorded while the Condition fails, and takes `timed_out`; an
// Event both pass takes `arrived`.
func TestTheTimeoutSkipsAnEventThatFailsTheFilterOrTheCondition(t *testing.T) {
	for name, c := range map[string]struct {
		condition string
		total     float64
		takes     func(w waiting) automationstep.ID
	}{
		"a large order":         {`{"kind":"tag","name":"vip","operator":"doesNotHave"}`, 150, func(w waiting) automationstep.ID { return w.arrived }},
		"a small order":         {`{"kind":"tag","name":"vip","operator":"doesNotHave"}`, 50, func(w waiting) automationstep.ID { return w.timedOut }},
		"its condition failing": {`{"kind":"tag","name":"vip","operator":"has"}`, 150, func(w waiting) automationstep.ID { return w.timedOut }},
	} {
		t.Run(name, func(t *testing.T) {
			w := awaiting(t, "trial.started", automationstep.AwaitedEvent{
				Matchers: []eventmatcher.Matcher{orderOver100(t)}, Condition: waitCondition(t, c.condition),
			}, nil)
			contact, runID := w.enroll(t, "trial.started")
			deadline := w.clock.Now().Add(waitTimeout)

			w.clock.Set(deadline.Add(5 * time.Minute))
			// Recorded within the window, its trigger job not yet run.
			order := collected(contact.ID, "order.completed", deadline.Add(-time.Minute), map[string]any{"total": c.total})
			require.NoError(t, w.env.Publisher().Publish(t.Context(), order))
			w.timeout(t, runID)

			assert.Equal(t, c.takes(w), w.standsOn(t, runID))
		})
	}
}
