package automations_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Branch path's and a Wait for event's Condition test the Event an earlier Wait for event arrived
// on, which the Enrollment keeps under its name (ADR 0136), by a property test whose subject is
// `awaited` (ADR 0181).

// The Steps of the versions the save tests write.
const (
	purchaseID automationstep.ID = "6b1e4d7f-9c2d-4f4b-8a8e-000000000001"
	reviewID   automationstep.ID = "6b1e4d7f-9c2d-4f4b-8a8e-000000000002"
	judgeID    automationstep.ID = "6b1e4d7f-9c2d-4f4b-8a8e-000000000003"
)

// awaitedOver100 is a test that the property total of the Event the Wait for event name arrived on
// is over 100.
func awaitedOver100(name string) string {
	return `{"kind":"property","of":{"kind":"awaited","name":"` + name + `"},"key":"total","test":{"kind":"number","operator":"gt","value":100}}`
}

// waitNamed is the Wait for event id, named name, for an order.created, its Condition the leaves
// written as JSON (none for no Condition), its `arrived` path holding the Steps given.
func waitNamed(t *testing.T, id automationstep.ID, name string, cond []string, arrived ...automationstep.Step) automationstep.Step {
	t.Helper()
	awaits := automationstep.AwaitedEvent{Matchers: []eventmatcher.Matcher{{Action: awaitedAction}}}
	if len(cond) > 0 {
		awaits.Condition = waitCondition(t, cond...)
	}
	return automationstep.Step{ID: id, Payload: automationstep.WaitForEvent{
		Name: name, TimeoutSeconds: int(waitTimeout / time.Second), Awaits: awaits,
		Arrived: automationstep.Root(arrived...), TimedOut: automationstep.Root(),
	}}
}

// judging is the Branch judgeID whose one path holds the leaves written as JSON.
func judging(t *testing.T, leaves ...string) automationstep.Step {
	t.Helper()
	return automationstep.Step{ID: judgeID, Payload: automationstep.Branch{
		Paths:     []automationstep.BranchPath{{Condition: conditiontree.Branch{Group: groupOf(t, leaves...)}, Steps: []automationstep.Step{}}},
		Otherwise: automationstep.Root(),
	}}
}

// A Branch or a Wait for event may test the Event of a Wait for event that can come before it: one
// earlier in its path, or one whose `arrived` path holds it.
func TestATestOfTheAwaitedEventNamesAWaitForEventBeforeIt(t *testing.T) {
	env := testhelper.Setup(t)
	for name, steps := range map[string]automationstep.Path{
		"a Branch after the Wait for event": automationstep.Root(
			waitNamed(t, purchaseID, "purchase", nil), judging(t, awaitedOver100("purchase"))),
		"a Branch on the Wait for event's arrived path": automationstep.Root(
			waitNamed(t, purchaseID, "purchase", nil, judging(t, awaitedOver100("purchase")))),
		"a later Wait for event's Condition": automationstep.Root(
			waitNamed(t, purchaseID, "purchase", nil), waitNamed(t, reviewID, "review", []string{awaitedOver100("purchase")})),
	} {
		t.Run(name, func(t *testing.T) {
			err := saveVersion(t, env, steps)

			require.NoError(t, err)
		})
	}
}

// A save is refused, naming the Step, when a test of the awaited Event names no Wait for event that
// can come before the Branch or the Wait for event holding it: a later one, the Wait for event
// itself, or none of the version.
func TestATestOfTheAwaitedEventNamingNoWaitForEventBeforeItIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	for name, c := range map[string]struct {
		steps automationstep.Path
		names string
	}{
		"a Branch before the Wait for event": {
			automationstep.Root(judging(t, awaitedOver100("purchase")), waitNamed(t, purchaseID, "purchase", nil)),
			"step " + string(judgeID) + ` (branch): path 1: the awaited event test names "purchase", which is no wait for event before the branch`,
		},
		"a Branch naming no Wait for event of the version": {
			automationstep.Root(waitNamed(t, purchaseID, "purchase", nil), judging(t, awaitedOver100("refund"))),
			"step " + string(judgeID) + ` (branch): path 1: the awaited event test names "refund", which is no wait for event before the branch`,
		},
		"a Wait for event naming itself": {
			automationstep.Root(waitNamed(t, purchaseID, "purchase", []string{awaitedOver100("purchase")})),
			"step " + string(purchaseID) + ` (wait_for_event): the condition: the awaited event test names "purchase", which is no wait for event before it`,
		},
		"a Wait for event naming a later one": {
			automationstep.Root(waitNamed(t, purchaseID, "purchase", []string{awaitedOver100("review")}), waitNamed(t, reviewID, "review", nil)),
			"step " + string(purchaseID) + ` (wait_for_event): the condition: the awaited event test names "review", which is no wait for event before it`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := saveVersion(t, env, c.steps)

			require.ErrorIs(t, err, automations.ErrInvalidStep)
			require.ErrorContains(t, err, c.names)
		})
	}
}

// orderOf records the Contact's order.created of the total, occurred now.
func (f awaitedFlow) orderOf(t *testing.T, c *ent.Contact, total float64) {
	t.Helper()
	handStored(t, f.env, c.ID, awaitedAction, collected(c.ID, awaitedAction, f.clock.Now(), map[string]any{"total": total}))
}

// recordOf is the Enrollment's record of the Step the version's root path holds at i.
func (f awaitedFlow) recordOf(t *testing.T, runID int64, i int) (*ent.AutomationStepRecord, automationstep.Step) {
	t.Helper()
	ctx, s := t.Context(), f.env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](f.env)
	live, err := m.Get(ctx, s, f.automationID)
	require.NoError(t, err)
	version, _ := automations.LiveAndDraft(live)
	st := version.Steps.Steps[i]
	records, err := m.StepRecords(ctx, s, f.automationID, runID)
	require.NoError(t, err)
	of := lo.Filter(records, func(r *ent.AutomationStepRecord, _ int) bool { return r.StepID == st.ID })
	require.Len(t, of, 1)
	return of[0], st
}

// A Branch testing that the awaited order's total is over 100 routes an order of 150 to its path and
// one of 50 to `otherwise`; after `timed_out` there is no copy, the test fails and `otherwise` is
// taken. The Step record keeps each path's outcome, never the value tested.
func TestABranchRoutesByTheAwaitedEvent(t *testing.T) {
	for name, c := range map[string]struct {
		order   func(t *testing.T, f awaitedFlow, c *ent.Contact)
		listed  bool
		outcome conditiontree.Truth
	}{
		"an order of 150": {func(t *testing.T, f awaitedFlow, c *ent.Contact) {
			t.Helper()
			f.clock.Advance(time.Minute)
			f.orderOf(t, c, 150)
		}, true, conditiontree.TruthTrue},
		"an order of 50": {func(t *testing.T, f awaitedFlow, c *ent.Contact) {
			t.Helper()
			f.clock.Advance(time.Minute)
			f.orderOf(t, c, 50)
		}, false, conditiontree.TruthFalse},
		"no order": {func(_ *testing.T, f awaitedFlow, _ *ent.Contact) {
			f.clock.Advance(waitTimeout)
		}, false, conditiontree.TruthFalse},
	} {
		t.Run(name, func(t *testing.T) {
			f := awaitThen(t, judging(t, awaitedOver100("purchase")).Payload)
			contact := enrollable(t, f.env.DB.Scoped(fixtures.AcmeID), 0)
			runID := f.enroll(t, contact)

			c.order(t, f, contact)
			f.run(t)

			record, st := f.recordOf(t, runID, 1)
			b := st.Payload.(automationstep.Branch)
			took := lo.FromPtr(record.PathID)
			want := lo.Ternary(c.listed, b.Paths[0].ID, b.Otherwise.ID)
			assert.Equal(t, want, took)
			assert.Equal(t, map[automationstep.ID]conditiontree.Truth{b.Paths[0].ID: c.outcome}, record.PathOutcomes)
		})
	}
}

// A Wait for event whose Condition tests the order an earlier Wait for event arrived on takes
// `arrived` only when the test holds: after an order of 150 a review takes `arrived`, after one of 50
// the review leaves the Enrollment waiting.
func TestAWaitForEventsConditionReadsTheEarlierAwaitedEvent(t *testing.T) {
	const reviewed = "review.posted"
	for name, c := range map[string]struct {
		total   float64
		arrived bool
	}{
		"an order of 150": {150, true},
		"an order of 50":  {50, false},
	} {
		t.Run(name, func(t *testing.T) {
			review := automationstep.WaitForEvent{
				Name: "review", TimeoutSeconds: int(waitTimeout / time.Second),
				Awaits: automationstep.AwaitedEvent{
					Matchers:  []eventmatcher.Matcher{{Action: reviewed}},
					Condition: waitCondition(t, awaitedOver100("purchase")),
				},
				Arrived: automationstep.Root(), TimedOut: automationstep.Root(),
			}
			f := awaitThen(t, review)
			contact := enrollable(t, f.env.DB.Scoped(fixtures.AcmeID), 0)
			runID := f.enroll(t, contact)
			f.clock.Advance(time.Minute)
			f.orderOf(t, contact, c.total)

			f.clock.Advance(time.Minute)
			handStored(t, f.env, contact.ID, reviewed, collected(contact.ID, reviewed, f.clock.Now(), map[string]any{"stars": 5}))

			record, st := f.recordOf(t, runID, 1)
			took := lo.FromPtr(record.PathID)
			want := lo.Ternary(c.arrived, st.Payload.(automationstep.WaitForEvent).Arrived.ID, "")
			assert.Equal(t, want, took)
		})
	}
}
