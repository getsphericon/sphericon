package automationstep_test

import (
	"errors"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
)

// stepsGen draws a version's Steps, each told apart by its wait.
func stepsGen() *rapid.Generator[[]automationstep.Step] {
	return rapid.Custom(func(t *rapid.T) []automationstep.Step {
		n := rapid.IntRange(0, 8).Draw(t, "n")
		steps := make([]automationstep.Step, n)
		for i := range steps {
			steps[i] = automationstep.Step{Type: automationstep.Wait, Seconds: 60 * (i + 1)}
		}
		return steps
	})
}

func TestAllVisitsEveryStepInOrderWithItsPosition(t *testing.T) {
	steps := []automationstep.Step{
		{Type: automationstep.Email, Subject: "Hello"},
		{Type: automationstep.Wait, Seconds: 60},
		{Type: automationstep.ApplyTag, Tag: "vip"},
	}
	var positions []int
	var kinds []automationstep.Kind
	for pos, st := range automationstep.All(steps) {
		positions = append(positions, pos)
		kinds = append(kinds, st.Type)
	}
	assert.Equal(t, []int{0, 1, 2}, positions)
	assert.Equal(t, []automationstep.Kind{automationstep.Email, automationstep.Wait, automationstep.ApplyTag}, kinds)
}

func TestAllStopsWhenTheVisitorStops(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		steps := stepsGen().Filter(func(s []automationstep.Step) bool { return len(s) > 0 }).Draw(rt, "steps")
		stop := rapid.IntRange(0, len(steps)-1).Draw(rt, "stop")
		assert.Equal(rt, stop+1, visitUntil(steps, stop))
	})
}

func TestAllOfNoStepsVisitsNothing(t *testing.T) {
	for range automationstep.All(nil) {
		t.Fatal("visited a Step of no Steps")
	}
}

func TestAtFindsTheStepAtAPosition(t *testing.T) {
	steps := []automationstep.Step{
		{Type: automationstep.Wait, Seconds: 60},
		{Type: automationstep.ApplyTag, Tag: "vip"},
	}
	st, ok := automationstep.At(steps, 1)
	assert.True(t, ok)
	assert.Equal(t, automationstep.Step{Type: automationstep.ApplyTag, Tag: "vip"}, *st)
}

// A position before the first Step or past the last names none: an Enrollment past the last Step
// has finished.
func TestAtFindsNothingOutsideTheSteps(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		steps := stepsGen().Draw(rt, "steps")
		pos := rapid.OneOf(rapid.IntRange(-5, -1), rapid.IntRange(len(steps), len(steps)+5)).Draw(rt, "pos")
		_, ok := automationstep.At(steps, pos)
		assert.False(rt, ok)
	})
}

func TestMapRewritesEachStepInPlace(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		steps := stepsGen().Draw(rt, "steps")
		held := slices.Clone(steps)
		positions := []int{}
		out, err := automationstep.Map(steps, func(pos int, st *automationstep.Step) (automationstep.Step, error) {
			positions = append(positions, pos)
			return automationstep.Step{Type: automationstep.ApplyTag, Tag: strconv.Itoa(st.Seconds)}, nil
		})
		require.NoError(rt, err)
		want := make([]automationstep.Step, len(steps))
		wantPositions := make([]int, len(steps))
		for i := range steps {
			want[i] = automationstep.Step{Type: automationstep.ApplyTag, Tag: strconv.Itoa(60 * (i + 1))}
			wantPositions[i] = i
		}
		assert.Equal(rt, want, out)
		assert.Equal(rt, wantPositions, positions)
		assert.Equal(rt, held, steps, "the Steps mapped are left as they were")
	})
}

// No Steps map to an empty, not a missing, list: a version stores `[]`.
func TestMapOfNoStepsIsEmpty(t *testing.T) {
	out, err := automationstep.Map(nil, func(int, *automationstep.Step) (automationstep.Step, error) {
		return automationstep.Step{}, nil
	})
	require.NoError(t, err)
	assert.NotNil(t, out)
	assert.Empty(t, out)
}

func TestMapStopsAtTheFirstError(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		steps := stepsGen().Filter(func(s []automationstep.Step) bool { return len(s) > 0 }).Draw(rt, "steps")
		failAt := rapid.IntRange(0, len(steps)-1).Draw(rt, "failAt")
		out, visited, err := mapFailingAt(steps, failAt)
		require.ErrorIs(rt, err, errRefused)
		assert.Nil(rt, out)
		assert.Equal(rt, failAt+1, visited)
	})
}

func TestRewriteRewritesEachStepInPlace(t *testing.T) {
	steps := []automationstep.Step{
		{Type: automationstep.Wait, Seconds: 60},
		{Type: automationstep.Wait, Seconds: 120},
	}
	out := automationstep.Rewrite(steps, func(pos int, st *automationstep.Step) automationstep.Step {
		return automationstep.Step{Type: automationstep.ApplyTag, Tag: strconv.Itoa(pos) + "/" + strconv.Itoa(st.Seconds)}
	})
	assert.Equal(t, []automationstep.Step{
		{Type: automationstep.ApplyTag, Tag: "0/60"},
		{Type: automationstep.ApplyTag, Tag: "1/120"},
	}, out)
	assert.NotNil(t, automationstep.Rewrite(nil, func(int, *automationstep.Step) automationstep.Step {
		return automationstep.Step{}
	}))
}

// visitUntil visits the Steps until the one at stop and returns how many it visited.
func visitUntil(steps []automationstep.Step, stop int) int {
	visited := 0
	for pos := range automationstep.All(steps) {
		visited++
		if pos == stop {
			break
		}
	}
	return visited
}

var errRefused = errors.New("refused")

// mapFailingAt maps the Steps with a rewrite refusing the one at failAt, and returns how many
// Steps it was given.
func mapFailingAt(steps []automationstep.Step, failAt int) ([]automationstep.Step, int, error) {
	visited := 0
	out, err := automationstep.Map(steps, func(pos int, st *automationstep.Step) (automationstep.Step, error) {
		visited++
		if pos == failAt {
			return automationstep.Step{}, errRefused
		}
		return *st, nil
	})
	return out, visited, err
}
