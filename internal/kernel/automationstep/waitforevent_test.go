package automationstep_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
)

// storedWait is a stored root path whose second Step is a Wait for event on order.created with a
// send in its arrived path and a tag in its timed out path, followed by an email both rejoin on.
const storedWait = `{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a01","steps":[` +
	`{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a02","type":"wait","seconds":60},` +
	`{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a03","type":"wait_for_event","name":"purchase","timeoutSeconds":259200,` +
	`"awaits":{"kind":"event","actions":["order.created","call.booked"]},` +
	`"arrived":{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a04","steps":[{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a05","type":"email","subject":"Thanks","body":"<p>Thanks</p>"}]},` +
	`"timedOut":{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a06","steps":[{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a07","type":"apply_tag","tag":"lapsed"}]}},` +
	`{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1a08","type":"email","subject":"Next","body":"<p>Next</p>"}]}`

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
	assert.Equal(t, automationstep.AwaitedEvent{Actions: []string{"order.created", "call.booked"}}, p.Steps[1].Payload.(automationstep.WaitForEvent).Awaits)
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
		"an unknown field": `{"type":"wait_for_event","name":"a","timeoutSeconds":1,"awaits":{"kind":"event","actions":["x"]},"arrived":{"id":"","steps":[]},"timedOut":{"id":"","steps":[]},"seconds":3}`,
		"an awaited field": `{"type":"wait_for_event","name":"a","timeoutSeconds":1,"awaits":{"kind":"event","actions":["x"],"step":"y"},"arrived":{"id":"","steps":[]},"timedOut":{"id":"","steps":[]}}`,
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

// storedMessageWait is a stored root path of an email, then a Wait for event on an open of its
// message, whose unknown path tags the Contact.
const storedMessageWait = `{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1b01","steps":[` +
	`{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1b02","type":"email","subject":"Welcome","body":"<p>Hello</p>"},` +
	`{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1b03","type":"wait_for_event","name":"welcome_open","timeoutSeconds":172800,` +
	`"awaits":{"kind":"message","step":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1b02","engagement":"opened"},` +
	`"arrived":{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1b04","steps":[]},` +
	`"timedOut":{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1b05","steps":[]},` +
	`"unknown":{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1b06","steps":[{"id":"6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1b07","type":"apply_tag","tag":"untracked"}]}}]}`

// A Wait for event on an open of a message is stored with the send Step it names, opened or
// clicked, and its unknown path, read back as it was; the walker visits its unknown path last and
// places a Step of it under `unknown`.
func TestAWaitForAMessageRoundTripsWithItsUnknownPath(t *testing.T) {
	var p automationstep.Path
	require.NoError(t, json.Unmarshal([]byte(storedMessageWait), &p))

	locations := automationstep.Locations(p)
	written, err := json.Marshal(p)
	require.NoError(t, err)
	w := p.Steps[1].Payload.(automationstep.WaitForEvent)

	assert.Equal(t, automationstep.AwaitedMessage{Step: "6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1b02", Engagement: conditiontree.MessageOpened}, w.Awaits)
	assert.Equal(t, "steps.1.unknown.steps.0", locations["6a0c6a52-4f0e-4f65-9a70-0d4a1c0e1b07"])
	assert.JSONEq(t, storedMessageWait, string(written))
}
