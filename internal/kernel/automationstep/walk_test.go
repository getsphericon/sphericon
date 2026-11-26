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
func stepsGen() *rapid.Generator[automationstep.Path] {
	return rapid.Custom(func(t *rapid.T) automationstep.Path {
		n := rapid.IntRange(0, 8).Draw(t, "n")
		steps := make([]automationstep.Step, n)
		for i := range steps {
			steps[i] = automationstep.New(automationstep.Wait{Seconds: 60 * (i + 1)})
		}
		return automationstep.Root(steps...)
	})
}

// seconds is the wait of a Step stepsGen drew.
func seconds(st *automationstep.Step) int { return st.Payload.(automationstep.Wait).Seconds }

func TestAllVisitsEveryStepInOrderWithItsPosition(t *testing.T) {
	steps := automationstep.Root(
		automationstep.New(automationstep.Email{Subject: "Hello"}),
		automationstep.New(automationstep.Wait{Seconds: 60}),
		automationstep.New(automationstep.ApplyTag{Tag: "vip"}),
	)
	var positions []int
	var kinds []automationstep.Kind
	for pos, st := range automationstep.All(steps) {
		positions = append(positions, pos)
		kinds = append(kinds, st.Kind())
	}
	assert.Equal(t, []int{0, 1, 2}, positions)
	assert.Equal(t, []automationstep.Kind{automationstep.KindEmail, automationstep.KindWait, automationstep.KindApplyTag}, kinds)
}

func TestAllStopsWhenTheVisitorStops(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		steps := stepsGen().Filter(func(p automationstep.Path) bool { return len(p.Steps) > 0 }).Draw(rt, "steps")
		stop := rapid.IntRange(0, len(steps.Steps)-1).Draw(rt, "stop")
		want := stop + 1
		visited := visitUntil(steps, stop)
		assert.Equal(rt, want, visited)
	})
}

func TestAllOfNoStepsVisitsNothing(t *testing.T) {
	for range automationstep.All(automationstep.Path{}) {
		t.Fatal("visited a Step of no Steps")
	}
}

// withIDs gives each Step of the path a distinct id, as a stored version holds them.
func withIDs(p automationstep.Path) automationstep.Path {
	for i := range p.Steps {
		p.Steps[i].ID = stepID(i)
	}
	return p
}

func TestFirstIsTheFirstStepOfThePath(t *testing.T) {
	steps := withIDs(automationstep.Root(automationstep.New(automationstep.Wait{Seconds: 60}), automationstep.New(automationstep.ApplyTag{Tag: "vip"})))
	st, ok := automationstep.First(steps)
	require.True(t, ok)
	assert.Equal(t, automationstep.ID("step-0"), st.ID)
	_, ok = automationstep.First(automationstep.Root())
	assert.False(t, ok, "a path of no Steps has no first Step")
}

func TestFindFindsTheStepOfAnID(t *testing.T) {
	tag := automationstep.New(automationstep.ApplyTag{Tag: "vip"})
	steps := withIDs(automationstep.Root(automationstep.New(automationstep.Wait{Seconds: 60}), tag))
	index := automationstep.NewIndex(steps)
	st, ok := index.Step("step-1")
	require.True(t, ok)
	assert.Equal(t, automationstep.Step{ID: "step-1", Payload: tag.Payload}, *st)
	_, ok = index.Step("step-9")
	assert.False(t, ok, "an id the version does not hold names no Step")
	_, ok = index.Step("")
	assert.False(t, ok)
}

// The Step after another is the following Step of the root path; the last Step has none, where an
// Enrollment finishes, and neither has an id the path does not hold.
func TestNextIsTheFollowingStepOfThePath(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		steps := withIDs(stepsGen().Filter(func(p automationstep.Path) bool { return len(p.Steps) > 1 }).Draw(rt, "steps"))
		last := len(steps.Steps) - 1
		pos := rapid.IntRange(0, last-1).Draw(rt, "pos")
		index := automationstep.NewIndex(steps)
		next, ok := index.Next(stepID(pos))
		require.True(rt, ok)
		assert.Equal(rt, stepID(pos+1), next.ID)
		assert.Equal(rt, 60*(pos+2), seconds(next))
		_, ok = index.Next(stepID(last))
		assert.False(rt, ok, "the last Step has no next")
	})
	single := withIDs(automationstep.Root(automationstep.New(automationstep.Wait{Seconds: 60})))
	index := automationstep.NewIndex(single)
	_, ok := index.Next("step-0")
	assert.False(t, ok, "the only Step is the last")
	_, ok = index.Next("elsewhere")
	assert.False(t, ok)
}

func TestMapRewritesEachStepInPlace(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		steps := stepsGen().Draw(rt, "steps")
		held := automationstep.Path{ID: steps.ID, Steps: slices.Clone(steps.Steps)}
		positions := []int{}
		out, err := automationstep.Map(steps, func(pos int, st *automationstep.Step) (automationstep.Step, error) {
			positions = append(positions, pos)
			return automationstep.Step{ID: st.ID, Payload: automationstep.ApplyTag{Tag: strconv.Itoa(seconds(st))}}, nil
		})
		require.NoError(rt, err)
		want := make([]automationstep.Step, len(steps.Steps))
		wantPositions := make([]int, len(steps.Steps))
		for i := range steps.Steps {
			want[i] = automationstep.Step{ID: held.Steps[i].ID, Payload: automationstep.ApplyTag{Tag: strconv.Itoa(seconds(&held.Steps[i]))}}
			wantPositions[i] = i
		}
		assert.Equal(rt, automationstep.Path{ID: held.ID, Steps: want}, out)
		assert.Equal(rt, wantPositions, positions)
		assert.Equal(rt, held, steps, "the Steps mapped are left as they were")
	})
}

// No Steps map to an empty, not a missing, list: a path stores `[]`.
func TestMapOfNoStepsIsEmpty(t *testing.T) {
	out, err := automationstep.Map(automationstep.Path{}, func(int, *automationstep.Step) (automationstep.Step, error) {
		return automationstep.Step{}, nil
	})
	require.NoError(t, err)
	assert.NotNil(t, out.Steps)
	assert.Empty(t, out.Steps)
}

func TestMapStopsAtTheFirstError(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		steps := stepsGen().Filter(func(p automationstep.Path) bool { return len(p.Steps) > 0 }).Draw(rt, "steps")
		failAt := rapid.IntRange(0, len(steps.Steps)-1).Draw(rt, "failAt")
		want := failAt + 1
		out, visited, err := mapFailingAt(steps, failAt)
		require.ErrorIs(rt, err, errRefused)
		assert.Equal(rt, automationstep.Path{}, out)
		assert.Equal(rt, want, visited)
	})
}

func TestRewriteRewritesEachStepInPlace(t *testing.T) {
	steps := automationstep.Path{ID: "root", Steps: []automationstep.Step{
		automationstep.New(automationstep.Wait{Seconds: 60}),
		automationstep.New(automationstep.Wait{Seconds: 120}),
	}}
	out := automationstep.Rewrite(steps, func(pos int, st *automationstep.Step) automationstep.Step {
		return automationstep.New(automationstep.ApplyTag{Tag: strconv.Itoa(pos) + "/" + strconv.Itoa(seconds(st))})
	})
	assert.Equal(t, automationstep.Path{ID: "root", Steps: []automationstep.Step{
		automationstep.New(automationstep.ApplyTag{Tag: "0/60"}),
		automationstep.New(automationstep.ApplyTag{Tag: "1/120"}),
	}}, out)
	none := automationstep.Rewrite(automationstep.Path{}, func(int, *automationstep.Step) automationstep.Step {
		return automationstep.Step{}
	})
	assert.NotNil(t, none.Steps)
}

// visitUntil visits the Steps until the one at stop and returns how many it visited.
func visitUntil(steps automationstep.Path, stop int) int {
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
func mapFailingAt(steps automationstep.Path, failAt int) (automationstep.Path, int, error) {
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

// An End finishes its path (ADR 0124): the first Step after one is found, and a path whose End is
// its last Step, or that holds none, has none.
func TestAfterEndFindsTheFirstStepThatFollowsAnEnd(t *testing.T) {
	end := automationstep.Step{ID: "e", Payload: automationstep.End{}}
	wait := automationstep.Step{ID: "w", Payload: automationstep.Wait{Seconds: 60}}
	tag := automationstep.Step{ID: "t", Payload: automationstep.ApplyTag{Tag: "vip"}}

	after, found := automationstep.AfterEnd(automationstep.Root(wait, end, tag, end))
	_, endLast := automationstep.AfterEnd(automationstep.Root(wait, tag, end))
	_, noEnd := automationstep.AfterEnd(automationstep.Root(wait, tag))

	require.True(t, found)
	assert.Equal(t, automationstep.ID("t"), after.ID)
	assert.False(t, endLast)
	assert.False(t, noEnd)
}

// stepID is the id withIDs gives the Step at pos.
func stepID(pos int) automationstep.ID { return automationstep.ID("step-" + strconv.Itoa(pos)) }

// Locations names each Step by where it lies in the path's document, as a request writes it: a
// refusal of the Step is placed there.
func TestLocationsNameEachStepByItsPlaceInTheDocument(t *testing.T) {
	step := func(id string, p automationstep.Payload) automationstep.Step {
		return automationstep.Step{ID: automationstep.ID(id), Payload: p}
	}
	wait := automationstep.Wait{Seconds: 60}
	branch := automationstep.Branch{
		Paths: []automationstep.BranchPath{
			{ID: "p1", Steps: []automationstep.Step{step("a", wait)}},
			{ID: "p2", Steps: []automationstep.Step{step("b", wait), step("c", wait)}},
		},
		Otherwise: automationstep.Path{ID: "o", Steps: []automationstep.Step{step("d", wait)}},
		Unknown:   &automationstep.Path{ID: "u", Steps: []automationstep.Step{step("e", wait)}},
	}
	split := automationstep.Split{Paths: []automationstep.SplitPath{
		{ID: "s1", Weight: 50, Steps: []automationstep.Step{}},
		{ID: "s2", Weight: 50, Steps: []automationstep.Step{step("f", automationstep.End{})}},
	}}
	steps := automationstep.Path{ID: "root", Steps: []automationstep.Step{
		step("first", wait), step("branch", branch), step("split", split),
	}}

	locations := automationstep.Locations(steps)

	assert.Equal(t, map[automationstep.ID]string{
		"first":  "steps.0",
		"branch": "steps.1",
		"a":      "steps.1.paths.0.steps.0",
		"b":      "steps.1.paths.1.steps.0",
		"c":      "steps.1.paths.1.steps.1",
		"d":      "steps.1.otherwise.steps.0",
		"e":      "steps.1.unknown.steps.0",
		"split":  "steps.2",
		"f":      "steps.2.paths.1.steps.0",
	}, locations)
}
