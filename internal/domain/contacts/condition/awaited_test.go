package condition_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// awaited is a test of the property key of the Event the Wait for event name arrived on, which the
// Enrollment keeps (ADR 0136).
func awaited(name, key string, test leaf) leaf {
	return leaf{"kind": "property", "of": leaf{"kind": "awaited", "name": name}, "key": key, "test": test}
}

// A test of the awaited Event reads the properties of the copy the Enrollment keeps under the Wait
// for event's name, in Postgres, two-valued: no copy, a copy of another Wait for event or no such
// property fails it, and so its negation holds. The triggering Event's properties are no copy.
func TestATestOfTheAwaitedEventReadsTheCopyKeptUnderItsName(t *testing.T) {
	over100 := awaited("purchase", "order.total", number("gt", 100))
	for name, c := range map[string]struct {
		awaited map[string]map[string]any
		trigger map[string]any
		want    []conditiontree.Truth
	}{
		"a total over the bound": {
			awaited: map[string]map[string]any{"purchase": {"order.total": 150}},
			want:    []conditiontree.Truth{conditiontree.TruthTrue, conditiontree.TruthFalse},
		},
		"a total under the bound": {
			awaited: map[string]map[string]any{"purchase": {"order.total": 50}},
			want:    []conditiontree.Truth{conditiontree.TruthFalse, conditiontree.TruthTrue},
		},
		"no copy": {
			want: []conditiontree.Truth{conditiontree.TruthFalse, conditiontree.TruthTrue},
		},
		"a copy without the property": {
			awaited: map[string]map[string]any{"purchase": {"product": "Lamp"}},
			want:    []conditiontree.Truth{conditiontree.TruthFalse, conditiontree.TruthTrue},
		},
		"a copy of another Wait for event": {
			awaited: map[string]map[string]any{"review": {"order.total": 150}},
			want:    []conditiontree.Truth{conditiontree.TruthFalse, conditiontree.TruthTrue},
		},
		"the triggering Event's property": {
			trigger: map[string]any{"order.total": 150},
			want:    []conditiontree.Truth{conditiontree.TruthFalse, conditiontree.TruthTrue},
		},
	} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)

			got, err := condition.New(time.Now).Outcomes(t.Context(), env.DB.Scoped(fixtures.MassiveID), condition.OutcomesInput{
				ContactID: ann, Conditions: []conditiontree.Branch{branchOf(over100), {Group: negated(over100)}},
				Properties: c.trigger, Awaited: c.awaited,
			})

			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

// A Wait for event's Condition reads the copy an earlier Wait for event's arrival left, as a
// Branch's does: it holds only when the test of it holds.
func TestAWaitForEventsConditionReadsTheAwaitedEvent(t *testing.T) {
	cond := conditiontree.WaitForEvent{Group: opsGroup(awaited("purchase", "order.total", number("gt", 100)))}
	for name, c := range map[string]struct {
		awaited map[string]map[string]any
		holds   bool
	}{
		"over the bound":  {map[string]map[string]any{"purchase": {"order.total": 150}}, true},
		"under the bound": {map[string]map[string]any{"purchase": {"order.total": 50}}, false},
		"no copy":         {nil, false},
	} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)

			holds, err := condition.New(time.Now).Holds(t.Context(), env.DB.Scoped(fixtures.MassiveID), condition.WaitForEventInput{
				ContactID: ann, Condition: cond, Awaited: c.awaited,
			})

			require.NoError(t, err)
			assert.Equal(t, c.holds, holds)
		})
	}
}

// A test of the awaited Event names its Wait for event: one that names none does not read.
func TestATestOfTheAwaitedEventNamesItsWaitForEvent(t *testing.T) {
	env := testhelper.Setup(t)
	nameless := leaf{"kind": "property", "of": leaf{"kind": "awaited"}, "key": "total", "test": number("gt", 100)}

	err := condition.New(time.Now).CheckBranch(t.Context(), env.DB.Scoped(fixtures.AcmeID), branchOf(nameless))

	require.ErrorContains(t, err, "names no wait for event")
}

// The Wait for event Steps a Condition's tests of the awaited Event name are listed at any depth and
// under a negation too, each once, in the tree's order: what a save checks against the Steps before
// the Condition, and what a Step reads of the Enrollment's copies.
func TestAwaitedNamesListsTheWaitForEventsAConditionReads(t *testing.T) {
	g := opsGroup(
		awaited("purchase", "total", number("gt", 100)),
		property("plan", text("eq", "pro")),
		negated(awaited("review", "stars", number("gte", 4)), awaited("purchase", "product", presence("set"))),
	)

	got := condition.AwaitedNames(g)

	assert.Equal(t, []string{"purchase", "review"}, got)
}
