package automationstep_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
)

// Every non-sending kind is independent of the send dependency. Inserting an email anywhere
// makes the sequence send, even when it contains HTTP calls or Tags around it.
func TestOnlyEmailStepsEstablishAnEmailDependency(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		others := rapid.SliceOf(rapid.SampledFrom([]automationstep.Step{
			automationstep.New(automationstep.Wait{Seconds: 60}),
			automationstep.New(automationstep.ApplyTag{Tag: "email"}),
			automationstep.New(automationstep.RemoveTag{Tag: "email"}),
			automationstep.New(automationstep.HTTP{Body: `{"type":"email"}`}),
		})).Draw(rt, "nonSending")
		position := rapid.IntRange(0, len(others)).Draw(rt, "position")
		sends := rapid.Bool().Draw(rt, "sends")
		middle := map[bool][]automationstep.Step{
			true:  {automationstep.New(automationstep.Email{Subject: "Hello"})},
			false: nil,
		}[sends]
		steps := append([]automationstep.Step{}, others[:position]...)
		steps = append(steps, middle...)
		steps = append(steps, others[position:]...)

		assert.Equal(rt, sends, automationstep.SendsEmail(automationstep.Root(steps...)))
	})
}

func TestNoStepsHaveNoEmailDependency(t *testing.T) {
	assert.False(t, automationstep.SendsEmail(automationstep.Path{}))
	assert.False(t, automationstep.SendsEmail(automationstep.Root()))
}

// A stored path is written with each Step's id and kind beside the fields of its kind only, and
// read back as it was.
func TestAStoredPathRoundTripsWithItsIDsAndKinds(t *testing.T) {
	stored := `{"id":"5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1a01","steps":[` +
		`{"id":"5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1a02","type":"email","subject":"Hi","body":"<p>Hi</p>","integrationId":7},` +
		`{"id":"5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1a03","type":"wait","seconds":60},` +
		`{"id":"5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1a04","type":"apply_tag","tag":"vip"},` +
		`{"id":"5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1a05","type":"http","url":"sealed","body":"{}"}]}`
	var p automationstep.Path
	require.NoError(t, json.Unmarshal([]byte(stored), &p))

	kinds := []automationstep.Kind{}
	for _, st := range automationstep.All(p) {
		kinds = append(kinds, st.Kind())
	}
	assert.Equal(t, "5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1a01", p.ID)
	assert.Equal(t, []automationstep.Kind{automationstep.KindEmail, automationstep.KindWait, automationstep.KindApplyTag, automationstep.KindHTTP}, kinds)
	assert.Equal(t, automationstep.Email{Subject: "Hi", Body: "<p>Hi</p>", IntegrationID: new(int64(7))}, p.Steps[0].Payload)
	written, err := json.Marshal(p)
	require.NoError(t, err)
	assert.JSONEq(t, stored, string(written))
}

// A stored Step is closed: a field its kind does not hold, another kind's included, and a kind
// nobody knows are refused rather than dropped.
func TestAStoredStepRefusesAFieldOfAnotherKindAndAnUnknownKind(t *testing.T) {
	for name, doc := range map[string]string{
		"another kind's field": `{"id":"5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1a02","type":"wait","seconds":60,"tag":"vip"}`,
		"an unknown field":     `{"id":"5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1a02","type":"apply_tag","tag":"vip","nope":1}`,
		"an unknown kind":      `{"id":"5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1a02","type":"fax"}`,
		"no kind":              `{"id":"5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1a02","seconds":60}`,
	} {
		t.Run(name, func(t *testing.T) {
			var st automationstep.Step
			err := json.Unmarshal([]byte(doc), &st)
			assert.Error(t, err)
		})
	}
}

// Each Step and the path without an id is given a new one; an id given is kept.
func TestIdentifyGivesAnIDToEachStepAndPathWithoutOne(t *testing.T) {
	kept := "5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1a02"
	minted := []string{"5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1aa1", "5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1aa2"}
	mint := func() string {
		id := minted[0]
		minted = minted[1:]
		return id
	}
	in := automationstep.Root(
		automationstep.Step{ID: kept, Payload: automationstep.Wait{Seconds: 60}},
		automationstep.New(automationstep.ApplyTag{Tag: "vip"}),
	)

	out, err := automationstep.Identify(in, mint)

	require.NoError(t, err)
	assert.Equal(t, "5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1aa1", out.ID)
	assert.Equal(t, kept, out.Steps[0].ID)
	assert.Equal(t, "5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1aa2", out.Steps[1].ID)
	assert.Empty(t, in.Steps[1].ID, "the path given is left as it was")
}

// An id that is not a UUID in its lowercase text form, and an id two Steps or a Step and the
// path share, are refused.
func TestIdentifyRefusesAMalformedOrRepeatedID(t *testing.T) {
	id := "5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1a02"
	mint := func() string { return "5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1aa1" }
	for name, p := range map[string]automationstep.Path{
		"not a uuid":         automationstep.Root(automationstep.Step{ID: "step-1", Payload: automationstep.Wait{}}),
		"upper case":         automationstep.Root(automationstep.Step{ID: "5B0C6A52-4F0E-4F65-9A70-0D4A1C0E1A02", Payload: automationstep.Wait{}}),
		"a malformed path":   {ID: "root", Steps: []automationstep.Step{}},
		"two Steps share it": automationstep.Root(automationstep.Step{ID: id, Payload: automationstep.Wait{}}, automationstep.Step{ID: id, Payload: automationstep.Wait{}}),
		"the path shares it": {ID: id, Steps: []automationstep.Step{{ID: id, Payload: automationstep.Wait{}}}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := automationstep.Identify(p, mint)
			assert.ErrorIs(t, err, automationstep.ErrBadID)
		})
	}
}
