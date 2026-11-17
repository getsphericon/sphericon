package automationstep_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
)

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

		assert.Equal(rt, sends, automationstep.SendsEmail(steps))
	})
}

func TestNoStepsHaveNoEmailDependency(t *testing.T) {
	assert.False(t, automationstep.SendsEmail(nil))
	assert.False(t, automationstep.SendsEmail([]automationstep.Step{}))
}
