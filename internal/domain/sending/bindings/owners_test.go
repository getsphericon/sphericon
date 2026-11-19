package bindings_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/schema"
	"github.com/getsphericon/sphericon/internal/domain/sending/bindings"
)

// A proven Identify hands a Visitor's Bindings over on every Channel (ADR 0179), so every Channel a
// Binding may have has the System event its hand-over records: a Channel added to the Binding's
// enum without its `<channel>.bound` in the contract would fail the hand-over of the first Visitor
// holding a Binding on it.
func TestEveryChannelOfBindingsHasItsBoundEvent(t *testing.T) {
	channels := bindingChannels()
	require.NotEmpty(t, channels)
	for _, ch := range channels {
		_, err := bindings.BoundAction(binding.Channel(ch))
		assert.NoError(t, err, ch)
	}
}

// A Channel the contract has no bound event of hands nothing over: the hand-over fails instead of
// recording an event no Trigger knows.
func TestAChannelWithoutABoundEventIsRefused(t *testing.T) {
	_, err := bindings.BoundAction(binding.Channel("pigeon"))
	require.Error(t, err)
}

// bindingChannels are the values of the Binding's channel enum, as the schema declares them.
func bindingChannels() []string {
	var channels []string
	for _, f := range (schema.Binding{}).Fields() {
		if d := f.Descriptor(); d.Name == binding.FieldChannel {
			for _, e := range d.Enums {
				channels = append(channels, e.V)
			}
		}
	}
	return channels
}
