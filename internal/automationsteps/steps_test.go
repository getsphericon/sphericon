package automationsteps_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/automationsteps"
)

func TestDecodeReadsTheStoredDefinition(t *testing.T) {
	steps, err := automationsteps.Decode(`[{"type":"email","subject":"Hi","body":"<p></p>"},{"type":"wait","seconds":60},{"type":"apply_tag","tag":"vip"}]`)
	require.NoError(t, err)
	assert.Equal(t, []automationsteps.Step{
		{Type: automationsteps.Email, Subject: "Hi", Body: "<p></p>"},
		{Type: automationsteps.Wait, Seconds: 60},
		{Type: automationsteps.ApplyTag, Tag: "vip"},
	}, steps)

	none, err := automationsteps.Decode("")
	require.NoError(t, err)
	assert.Empty(t, none, "an empty definition has no steps")

	_, err = automationsteps.Decode("{not json")
	require.Error(t, err)
}

// Every non-sending kind is independent of the send dependency. Inserting an email anywhere
// makes the sequence send, even when it contains HTTP calls or Tags around it.
func TestOnlyEmailStepsEstablishAnEmailDependency(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		others := rapid.SliceOf(rapid.SampledFrom([]automationsteps.Step{
			{Type: automationsteps.Wait, Seconds: 60},
			{Type: automationsteps.ApplyTag, Tag: "email"},
			{Type: automationsteps.RemoveTag, Tag: "email"},
			{Type: automationsteps.HTTP, Body: `{"type":"email"}`},
		})).Draw(rt, "nonSending")
		position := rapid.IntRange(0, len(others)).Draw(rt, "position")
		sends := rapid.Bool().Draw(rt, "sends")
		middle := map[bool][]automationsteps.Step{
			true:  {{Type: automationsteps.Email, Subject: "Hello"}},
			false: nil,
		}[sends]
		steps := append([]automationsteps.Step{}, others[:position]...)
		steps = append(steps, middle...)
		steps = append(steps, others[position:]...)
		stored, err := json.Marshal(steps)
		require.NoError(rt, err)

		actual, err := automationsteps.SendsEmail(string(stored))

		require.NoError(rt, err)
		assert.Equal(rt, sends, actual)
	})
}

func TestUnreadableDefinitionsCannotEstablishAnEmailDependency(t *testing.T) {
	sends, err := automationsteps.SendsEmail(`[{"type":"email"},`)
	require.Error(t, err)
	assert.False(t, sends)
}

func TestAnEmptyStoredDefinitionHasNoEmailDependency(t *testing.T) {
	sends, err := automationsteps.SendsEmail("")
	require.NoError(t, err)
	assert.False(t, sends)
}

func FuzzUnreadableStepsNeverEstablishASendingDependency(f *testing.F) {
	f.Add("")
	f.Add(`[{"type":"email","subject":"Hello"},{"type":"wait","seconds":60}]`)
	f.Add(`[{"type":"http","body":"email"}]`)
	f.Add(`[{"type":"email"},`)
	f.Fuzz(func(t *testing.T, definition string) {
		steps, decodeErr := automationsteps.Decode(definition)
		sends, sendingErr := automationsteps.SendsEmail(definition)

		assert.Equal(t, decodeErr != nil, sendingErr != nil)
		assert.True(t, decodeErr == nil || steps == nil, "damaged definitions expose no partial Steps")
		assert.True(t, sendingErr == nil || !sends, "an error must not establish a dependency")
	})
}
