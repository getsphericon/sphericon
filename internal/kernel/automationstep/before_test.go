package automationstep_test

import (
	"strconv"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

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

// treeGen draws a version's tree of email Steps and Branches, nested a few levels deep, each Step
// with an id of its own.
func treeGen() *rapid.Generator[automationstep.Path] {
	return rapid.Custom(func(t *rapid.T) automationstep.Path {
		next := 0
		return automationstep.Root(drawSteps(t, &next, 0)...)
	})
}

// drawSteps draws the Steps of one path at the depth, numbering their ids on from next.
func drawSteps(t *rapid.T, next *int, depth int) []automationstep.Step {
	n := rapid.IntRange(0, 3).Draw(t, "steps")
	steps := make([]automationstep.Step, 0, n)
	for range n {
		*next++
		id := automationstep.ID(strconv.Itoa(*next))
		if depth < 3 && rapid.Bool().Draw(t, "fork") {
			steps = append(steps, branch(id, drawSteps(t, next, depth+1), drawSteps(t, next, depth+1)))
			continue
		}
		steps = append(steps, step(id))
	}
	return steps
}

// within are the ids of the Steps of the paths of the Step st, at any depth.
func within(st *automationstep.Step) map[automationstep.ID]bool {
	out := map[automationstep.ID]bool{}
	for _, sub := range automationstep.Forks(st) {
		for _, inner := range automationstep.All(sub) {
			out[inner.ID] = true
		}
	}
	return out
}

// Whatever the tree, what may have been passed before a Step comes earlier in the walk and lies
// outside the Step's own paths, and what may have been passed before any of those may have been
// passed before the Step too: an Enrollment that passed a Step had passed what came before it.
func TestBeforeComesEarlierAndHoldsWhatCameBeforeIt(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		tree := treeGen().Draw(rt, "tree")
		positions := map[automationstep.ID]int{}
		for pos, st := range automationstep.All(tree) {
			positions[st.ID] = pos
		}
		for pos, st := range automationstep.All(tree) {
			before := automationstep.Before(tree, st.ID)
			passed := lo.SliceToMap(before, func(s *automationstep.Step) (automationstep.ID, bool) { return s.ID, true })
			own := within(st)
			earlier := lo.EveryBy(before, func(s *automationstep.Step) bool { return positions[s.ID] < pos && !own[s.ID] })
			closed := lo.EveryBy(before, func(s *automationstep.Step) bool {
				return lo.EveryBy(automationstep.Before(tree, s.ID), func(p *automationstep.Step) bool { return passed[p.ID] })
			})

			assert.True(rt, earlier, "step %s: every Step before it comes earlier, outside its own paths", st.ID)
			assert.True(rt, closed, "step %s: what came before a Step before it came before it too", st.ID)
		}
	})
}
