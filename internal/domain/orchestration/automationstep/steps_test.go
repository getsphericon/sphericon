package automationstep_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/automationstep"
)

func TestDecodeReadsTheStoredDefinition(t *testing.T) {
	steps, err := automationstep.Decode(`[{"type":"email","subject":"Hi","body":"<p></p>"},{"type":"wait","seconds":60},{"type":"apply_tag","tag":"vip"}]`)
	require.NoError(t, err)
	assert.Equal(t, []automationstep.Step{
		{Type: automationstep.Email, Subject: "Hi", Body: "<p></p>"},
		{Type: automationstep.Wait, Seconds: 60},
		{Type: automationstep.ApplyTag, Tag: "vip"},
	}, steps)

	none, err := automationstep.Decode("")
	require.NoError(t, err)
	assert.Empty(t, none, "an empty definition has no steps")

	_, err = automationstep.Decode("{not json")
	require.Error(t, err)
}

// Every non-sending kind is independent of the send dependency. Inserting an email anywhere
// makes the sequence send, even when it contains HTTP calls or Tags around it.
func TestOnlyEmailStepsEstablishAnEmailDependency(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		others := rapid.SliceOf(rapid.SampledFrom([]automationstep.Step{
			{Type: automationstep.Wait, Seconds: 60},
			{Type: automationstep.ApplyTag, Tag: "email"},
			{Type: automationstep.RemoveTag, Tag: "email"},
			{Type: automationstep.HTTP, Body: `{"type":"email"}`},
		})).Draw(rt, "nonSending")
		position := rapid.IntRange(0, len(others)).Draw(rt, "position")
		sends := rapid.Bool().Draw(rt, "sends")
		middle := map[bool][]automationstep.Step{
			true:  {{Type: automationstep.Email, Subject: "Hello"}},
			false: nil,
		}[sends]
		steps := append([]automationstep.Step{}, others[:position]...)
		steps = append(steps, middle...)
		steps = append(steps, others[position:]...)
		stored, err := json.Marshal(steps)
		require.NoError(rt, err)

		actual, err := automationstep.SendsEmail(string(stored))

		require.NoError(rt, err)
		assert.Equal(rt, sends, actual)
	})
}

func TestUnreadableDefinitionsCannotEstablishAnEmailDependency(t *testing.T) {
	sends, err := automationstep.SendsEmail(`[{"type":"email"},`)
	require.Error(t, err)
	assert.False(t, sends)
}

func TestAnEmptyStoredDefinitionHasNoEmailDependency(t *testing.T) {
	sends, err := automationstep.SendsEmail("")
	require.NoError(t, err)
	assert.False(t, sends)
}

func FuzzUnreadableStepsNeverEstablishASendingDependency(f *testing.F) {
	f.Add("")
	f.Add(`[{"type":"email","subject":"Hello"},{"type":"wait","seconds":60}]`)
	f.Add(`[{"type":"http","body":"email"}]`)
	f.Add(`[{"type":"email"},`)
	f.Fuzz(func(t *testing.T, definition string) {
		steps, decodeErr := automationstep.Decode(definition)
		sends, sendingErr := automationstep.SendsEmail(definition)

		assert.Equal(t, decodeErr != nil, sendingErr != nil)
		assert.True(t, decodeErr == nil || steps == nil, "damaged definitions expose no partial Steps")
		assert.True(t, sendingErr == nil || !sends, "an error must not establish a dependency")
	})
}
