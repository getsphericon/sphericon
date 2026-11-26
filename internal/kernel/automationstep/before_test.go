package automationstep_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
)

// step is a Step of the id, sending an email.
func step(id automationstep.ID) automationstep.Step {
	return automationstep.Step{ID: id, Payload: automationstep.Email{Subject: string(id)}}
}

// branch is a Branch of the id with one listed path and otherwise.
func branch(id automationstep.ID, listed, otherwise []automationstep.Step) automationstep.Step {
	return automationstep.Step{ID: id, Payload: automationstep.Branch{
		Paths:     []automationstep.BranchPath{{ID: id + "-yes", Steps: listed}},
		Otherwise: automationstep.Path{ID: id + "-no", Steps: otherwise},
	}}
}

// rejoined is a version whose Steps rejoin twice: A, then the Branch F1 of B or C, then D, then
// the Branch F2 of [E, then the Branch G of H or nothing] or [I, then the Branch X of K or
// nothing], then J.
var rejoined = automationstep.Root(
	step("A"),
	branch("F1", []automationstep.Step{step("B")}, []automationstep.Step{step("C")}),
	step("D"),
	branch("F2",
		[]automationstep.Step{step("E"), branch("G", []automationstep.Step{step("H")}, nil)},
		[]automationstep.Step{step("I"), branch("X", []automationstep.Step{step("K")}, nil)},
	),
	step("J"),
)

// idsOf are the ids of the Steps.
func idsOf(steps []*automationstep.Step) []automationstep.ID {
	out := make([]automationstep.ID, len(steps))
	for i, st := range steps {
		out[i] = st.ID
	}
	return out
}

// An Enrollment may have passed, when it reaches a Step, the Steps before it in its path and in
// every path around it, those of the forks it passed included, whichever path of them it took. It
// never passed a Step after it, one of its own paths, or one of another path of a fork it is in.
func TestBeforeIsWhatAnEnrollmentMayHavePassed(t *testing.T) {
	beforeX := idsOf(automationstep.Before(rejoined, "X"))
	beforeG := idsOf(automationstep.Before(rejoined, "G"))
	beforeH := idsOf(automationstep.Before(rejoined, "H"))
	beforeJ := idsOf(automationstep.Before(rejoined, "J"))
	beforeA := idsOf(automationstep.Before(rejoined, "A"))

	assert.Equal(t, []automationstep.ID{"A", "F1", "B", "C", "D", "F2", "I"}, beforeX)
	assert.Equal(t, []automationstep.ID{"A", "F1", "B", "C", "D", "F2", "E"}, beforeG)
	assert.Equal(t, []automationstep.ID{"A", "F1", "B", "C", "D", "F2", "E", "G"}, beforeH)
	assert.Equal(t, []automationstep.ID{"A", "F1", "B", "C", "D", "F2", "E", "G", "H", "I", "X", "K"}, beforeJ)
	assert.Empty(t, beforeA)
}

// A Step the version does not hold has nothing before it.
func TestBeforeAStepNotInTheVersionIsNothing(t *testing.T) {
	got := automationstep.Before(rejoined, "nowhere")

	assert.Empty(t, got)
}
