package consent_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/internal/consent"
)

var provenances = []confirmation.Provenance{
	confirmation.ProvenanceDoubleOptIn, confirmation.ProvenanceKeyword,
	confirmation.ProvenanceImported, confirmation.ProvenanceGrandfathered,
}

// The order of evidence (ADR 0077): the person's own act first, then consent collected elsewhere
// with its evidence, then a prior basis an admin asserted.
func TestTheStrongestProvenanceIsThePersonsOwnAct(t *testing.T) {
	assert.True(t, consent.Stronger(confirmation.ProvenanceDoubleOptIn, confirmation.ProvenanceKeyword))
	assert.True(t, consent.Stronger(confirmation.ProvenanceKeyword, confirmation.ProvenanceImported))
	assert.True(t, consent.Stronger(confirmation.ProvenanceImported, confirmation.ProvenanceGrandfathered))
}

// Stronger is a strict total order over the provenances: of two different ones exactly one is
// stronger, none is stronger than itself, and it is transitive, so which Confirmation stands never
// depends on the order they were recorded in.
func TestStrongerIsAStrictTotalOrder(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		pick := rapid.SampledFrom(provenances)
		a, b, c := pick.Draw(t, "a"), pick.Draw(t, "b"), pick.Draw(t, "c")

		aOverB, bOverA := consent.Stronger(a, b), consent.Stronger(b, a)
		chain := aOverB && consent.Stronger(b, c)
		ac := consent.Stronger(a, c)

		assert.Equal(t, a != b, aOverB != bOverA, "of two different provenances exactly one is stronger, of one none")
		assert.False(t, chain && !ac, "%s > %s > %s implies %s > %s", a, b, c, a, c)
	})
}
