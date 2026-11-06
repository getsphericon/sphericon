package confirmations_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
)

var provenances = []confirmation.Provenance{
	confirmation.ProvenanceDoubleOptIn, confirmation.ProvenanceKeyword,
	confirmation.ProvenanceImported, confirmation.ProvenanceGrandfathered,
}

// The order of evidence (ADR 0077): the person's own act first, then consent collected elsewhere
// with its evidence, then a prior basis an admin asserted.
func TestTheStrongestProvenanceIsThePersonsOwnAct(t *testing.T) {
	assert.True(t, confirmations.Stronger(confirmation.ProvenanceDoubleOptIn, confirmation.ProvenanceKeyword))
	assert.True(t, confirmations.Stronger(confirmation.ProvenanceKeyword, confirmation.ProvenanceImported))
	assert.True(t, confirmations.Stronger(confirmation.ProvenanceImported, confirmation.ProvenanceGrandfathered))
}

// Stronger is a strict total order over the provenances: of two different ones exactly one is
// stronger, none is stronger than itself, and it is transitive, so which Confirmation stands never
// depends on the order they were recorded in.
func TestStrongerIsAStrictTotalOrder(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		pick := rapid.SampledFrom(provenances)
		a, b, c := pick.Draw(t, "a"), pick.Draw(t, "b"), pick.Draw(t, "c")

		aOverB, bOverA := confirmations.Stronger(a, b), confirmations.Stronger(b, a)
		chain := aOverB && confirmations.Stronger(b, c)
		ac := confirmations.Stronger(a, c)

		assert.Equal(t, a != b, aOverB != bOverA, "of two different provenances exactly one is stronger, of one none")
		assert.False(t, chain && !ac, "%s > %s > %s implies %s > %s", a, b, c, a, c)
	})
}
