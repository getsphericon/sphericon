package automationstep_test

import (
	"encoding/json"
	"slices"
	"strconv"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
)

// splitTree is a version whose root holds a, then a Split s with a path v (x) of weight 30 and an
// empty holdout h of weight 70, then c:
//
//	a, s[v 30: x | h 70: -], c
func splitTree() automationstep.Path {
	split := automationstep.Step{ID: "s", Payload: automationstep.Split{Paths: []automationstep.SplitPath{
		{ID: "v", Weight: 30, Steps: []automationstep.Step{wait("x", 3)}},
		{ID: "h", Weight: 70, Steps: []automationstep.Step{}},
	}}}
	return automationstep.Path{ID: "root", Steps: []automationstep.Step{wait("a", 1), split, wait("c", 2)}}
}

// A Split's paths are walked after it, in the order it lists them, and rejoin at the Step after it:
// its empty holdout goes straight on.
func TestASplitsPathsRejoinAfterIt(t *testing.T) {
	index := automationstep.NewIndex(splitTree())

	visited := ids(automationstep.All(splitTree()))
	afterX, _ := index.Next("x")
	holdout, ok := index.Enter("h")

	assert.Equal(t, []automationstep.ID{"a", "s", "x", "c"}, visited)
	assert.Equal(t, automationstep.ID("c"), afterX.ID)
	require.True(t, ok)
	assert.Equal(t, automationstep.ID("c"), holdout.ID, "the holdout goes on to the Step after the Split")
}

// A rewrite reaches the Steps of a Split's paths and keeps each path's id and weight, in order.
func TestMapKeepsASplitsWeights(t *testing.T) {
	out := automationstep.Rewrite(splitTree(), func(_ int, st *automationstep.Step) automationstep.Step { return *st })

	s := out.Steps[1].Payload.(automationstep.Split)
	assert.Equal(t, []automationstep.SplitPath{
		{ID: "v", Weight: 30, Steps: []automationstep.Step{wait("x", 3)}},
		{ID: "h", Weight: 70, Steps: []automationstep.Step{}},
	}, s.Paths)
}

// Every path of a Split gets an id, after the Split and before its Steps.
func TestIdentifyGivesASplitsPathsIDs(t *testing.T) {
	split := automationstep.New(automationstep.Split{Paths: []automationstep.SplitPath{
		{Weight: 50, Steps: []automationstep.Step{automationstep.New(automationstep.Wait{Seconds: 1})}},
		{Weight: 50, Steps: []automationstep.Step{}},
	}})
	n := 0
	mint := func() automationstep.ID {
		n++
		return automationstep.ID("00000000-0000-4000-8000-" + strconv.Itoa(100000000000+n))
	}

	out, err := automationstep.Identify(automationstep.Root(split), mint)

	require.NoError(t, err)
	s := out.Steps[0].Payload.(automationstep.Split)
	assert.NotEmpty(t, s.Paths[0].ID)
	assert.NotEmpty(t, s.Paths[1].ID)
	assert.Equal(t, []int{50, 50}, []int{s.Paths[0].Weight, s.Paths[1].Weight})
	assert.Equal(t, 5, n, "the root, the Split, its two paths and the Step of its first path")
}

// A stored Split is written with its paths and weights and read back as it was; a field a Split
// does not hold is refused.
func TestAStoredSplitRoundTrips(t *testing.T) {
	stored, err := json.Marshal(splitTree())
	require.NoError(t, err)

	var back automationstep.Path
	require.NoError(t, json.Unmarshal(stored, &back))
	again, err := json.Marshal(back)
	require.NoError(t, err)
	var st automationstep.Step
	refused := json.Unmarshal([]byte(`{"id":"s","type":"split","paths":[],"otherwise":{"id":"o","steps":[]}}`), &st)

	assert.JSONEq(t, string(stored), string(again))
	assert.Contains(t, string(stored), `{"id":"v","weight":30,"steps":[`)
	require.Error(t, refused)
}

// A draw of 0 to 99 lands on the path whose share of the running total holds it: of 30 and 70,
// 0 to 29 are the first path's, 30 to 99 the second's.
func TestASplitLandsADrawOnTheRunningTotalOfItsWeights(t *testing.T) {
	s := splitTree().Steps[1].Payload.(automationstep.Split)
	cases := map[int]automationstep.ID{0: "v", 29: "v", 30: "h", 99: "h"}
	for draw, want := range cases {
		assert.Equal(t, want, s.At(draw), "draw %d", draw)
	}
}

// The draw of a Contact at a Split's Step for an Enrollment's ordinal is the first eight bytes of the
// SHA-256 of "<contact>:<step>:<ordinal>", big-endian, mod 100: the same wherever it is computed.
// The values were computed apart from this package (shasum), so a change of the hash, which would
// move running Enrollments' Contacts to other paths, is seen.
func TestTheDrawIsAHashOfTheContactTheStepAndTheOrdinal(t *testing.T) {
	step := automationstep.ID("3f1c0e9a-5b7d-4c2e-9a10-6d8e7f2b4c01")

	got := []int{
		automationstep.Draw(1, step, 1),
		automationstep.Draw(1, step, 2),
		automationstep.Draw(2, step, 1),
	}

	assert.Equal(t, []int{5, 3, 54}, got)
}

// Whatever the Contact, the Step and the ordinal, the draw is one of 0 to 99, and a Split of any
// weights summing to 100 lands it on one of its own paths.
func TestEveryDrawLandsOnAPathOfTheSplit(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		weights := weightsSummingTo100(t)
		paths := make([]automationstep.SplitPath, len(weights))
		for i, w := range weights {
			paths[i] = automationstep.SplitPath{ID: automationstep.ID("p" + strconv.Itoa(i)), Weight: w}
		}
		s := automationstep.Split{Paths: paths}
		draw := automationstep.Draw(rapid.Int64().Draw(t, "contact"), automationstep.ID(rapid.String().Draw(t, "step")), rapid.IntRange(1, 1000).Draw(t, "ordinal"))

		landed := s.At(draw)

		assert.GreaterOrEqual(t, draw, 0)
		assert.Less(t, draw, 100)
		assert.Contains(t, lo.Map(paths, func(p automationstep.SplitPath, _ int) automationstep.ID { return p.ID }), landed)
	})
}

// weightsSummingTo100 draws two to ten whole weights of at least 1 that sum to 100.
func weightsSummingTo100(t *rapid.T) []int {
	n := rapid.IntRange(2, 10).Draw(t, "paths")
	cuts := rapid.SliceOfNDistinct(rapid.IntRange(1, 99), n-1, n-1, rapid.ID[int]).Draw(t, "cuts")
	bounds := append(append([]int{0}, cuts...), 100)
	slices.Sort(bounds)
	weights := make([]int, 0, n)
	for i := 1; i < len(bounds); i++ {
		weights = append(weights, bounds[i]-bounds[i-1])
	}
	return weights
}
