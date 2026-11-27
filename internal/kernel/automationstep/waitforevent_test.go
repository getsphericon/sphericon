package automationstep_test

import (
	"encoding/json"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
)

// storedWait is a stored root path whose second Step is a Wait for event on an order.created over
// 100 or any call.booked, for a Contact tagged `vip`, with a send in its arrived path and a tag in its timed out path, followed by an email both rejoin on.
const storedWait = `{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a01","steps":[` +
	`{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a02","type":"wait","seconds":60},` +
	`{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a03","type":"wait_for_event","name":"purchase","timeoutSeconds":259200,` +
	`"awaits":{"kind":"event","matchers":[` + orderOver100 + `,{"action":"call.booked"}],` +
	`"condition":{"kind":"group","combinator":"and","entries":[{"kind":"tag","name":"vip","operator":"has"}]}},` +
	`"arrived":{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a04","steps":[{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a05","type":"email","subject":"Thanks","body":"<p>Thanks</p>"}]},` +
	`"timedOut":{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a06","steps":[{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a07","type":"apply_tag","tag":"lapsed"}]}},` +
	`{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a08","type":"email","subject":"Next","body":"<p>Next</p>"}]}`

// orderOver100 is the Event matcher of an order.created whose total is over 100.
const orderOver100 = `{"action":"order.created","filter":{"kind":"group","combinator":"and","entries":[` +
	`{"kind":"property","key":"total","test":{"kind":"number","operator":"gt","value":100}}]}}`

// A Wait for event is stored with its name, its timeout, what it awaits by kind and its two paths,
// and read back as it was; the walker visits its arrived path, then its timed out path, then the
// Step after it, and places a Step of each path under the path's field.
func TestAWaitForEventRoundTripsAndItsPathsAreWalked(t *testing.T) {
	var p automationstep.Path
	require.NoError(t, json.Unmarshal([]byte(storedWait), &p))

	kinds := []automationstep.Kind{}
	for _, st := range automationstep.All(p) {
		kinds = append(kinds, st.Kind())
	}
	locations := automationstep.Locations(p)
	written, err := json.Marshal(p)
	require.NoError(t, err)

	assert.Equal(t, []automationstep.Kind{
		automationstep.KindWait, automationstep.KindWaitForEvent, automationstep.KindEmail, automationstep.KindApplyTag, automationstep.KindEmail,
	}, kinds)
	awaited := p.Steps[1].Payload.(automationstep.WaitForEvent).Awaits.(automationstep.AwaitedEvent)
	assert.Equal(t, []string{"order.created", "call.booked"}, lo.Map(awaited.Matchers, func(m eventmatcher.Matcher, _ int) string { return m.Action }))
	assert.NotNil(t, awaited.Matchers[0].Filter, "the first matcher keeps its filter")
	assert.Nil(t, awaited.Matchers[1].Filter)
	assert.NotNil(t, awaited.Condition)
	assert.Equal(t, "steps.1.arrived.steps.0", locations["6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a05"])
	assert.Equal(t, "steps.1.timedOut.steps.0", locations["6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a07"])
	assert.JSONEq(t, storedWait, string(written))
}

// An Enrollment that takes either path of a Wait for event goes on to the Step after it once the
// path is done, and an empty path goes there at once.
func TestBothPathsOfAWaitForEventRejoinAfterIt(t *testing.T) {
	var p automationstep.Path
	require.NoError(t, json.Unmarshal([]byte(storedWait), &p))
	x := automationstep.NewIndex(p)

	afterSend, _ := x.Next("6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a05")
	afterTag, _ := x.Next("6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a07")
	entered, _ := x.Enter("6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a04")

	assert.Equal(t, automationstep.ID("6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a08"), afterSend.ID)
	assert.Equal(t, automationstep.ID("6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a08"), afterTag.ID)
	assert.Equal(t, automationstep.ID("6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a05"), entered.ID)
}

// A stored Wait for event is closed: a field it does not hold, a field its kind of awaiting does not
// hold and a kind nobody knows are refused.
func TestAStoredWaitForEventRefusesAFieldItDoesNotHold(t *testing.T) {
	for name, doc := range map[string]string{
		"an unknown field": `{"type":"wait_for_event","name":"a","timeoutSeconds":1,"awaits":{"kind":"event","matchers":[{"action":"x"}]},"arrived":{"id":"","steps":[]},"timedOut":{"id":"","steps":[]},"seconds":3}`,
		"an awaited field": `{"type":"wait_for_event","name":"a","timeoutSeconds":1,"awaits":{"kind":"event","matchers":[{"action":"x"}],"step":"y"},"arrived":{"id":"","steps":[]},"timedOut":{"id":"","steps":[]}}`,
		"an unknown kind":  `{"type":"wait_for_event","name":"a","timeoutSeconds":1,"awaits":{"kind":"reply"},"arrived":{"id":"","steps":[]},"timedOut":{"id":"","steps":[]}}`,
		"nothing awaited":  `{"type":"wait_for_event","name":"a","timeoutSeconds":1,"arrived":{"id":"","steps":[]},"timedOut":{"id":"","steps":[]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			var st automationstep.Step
			err := json.Unmarshal([]byte(doc), &st)
			assert.Error(t, err)
		})
	}
}
