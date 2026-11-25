package automationstep_test

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
)

// wait is a wait Step of the id, told apart by its seconds.
func wait(id automationstep.ID, seconds int) automationstep.Step {
	return automationstep.Step{ID: id, Payload: automationstep.Wait{Seconds: seconds}}
}

// vip is the Condition of a Branch path that tests the Tag "vip".
func vip() conditiontree.Branch {
	return conditiontree.Branch{Group: conditiontree.Group{Kind: conditiontree.KindGroup, Combinator: conditiontree.And,
		Entries: []json.RawMessage{json.RawMessage(`{"kind":"tag","name":"vip","operator":"has"}`)}}}
}

// tree is a version whose root holds a, then a Branch b with paths p1 (x, then a nested Branch n
// whose path q holds y and whose otherwise o2 is empty) and p2 (empty), its otherwise o (z) and its
// unknown u (empty), then c:
//
//	a, b[p1: x, n[q: y | o2: -] | p2: - | o: z | u: -], c
func tree() automationstep.Path {
	nested := automationstep.Step{ID: "n", Payload: automationstep.Branch{
		Paths:     []automationstep.BranchPath{{ID: "q", Condition: vip(), Steps: []automationstep.Step{wait("y", 6)}}},
		Otherwise: automationstep.Path{ID: "o2", Steps: []automationstep.Step{}},
	}}
	branch := automationstep.Step{ID: "b", Payload: automationstep.Branch{
		Paths: []automationstep.BranchPath{
			{ID: "p1", Condition: vip(), Steps: []automationstep.Step{wait("x", 3), nested}},
			{ID: "p2", Condition: vip(), Steps: []automationstep.Step{}},
		},
		Otherwise: automationstep.Path{ID: "o", Steps: []automationstep.Step{wait("z", 4)}},
		Unknown:   &automationstep.Path{ID: "u", Steps: []automationstep.Step{}},
	}}
	return automationstep.Path{ID: "root", Steps: []automationstep.Step{wait("a", 1), branch, wait("c", 2)}}
}

func ids(steps func(func(int, *automationstep.Step) bool)) []automationstep.ID {
	var out []automationstep.ID
	for _, st := range steps {
		out = append(out, st.ID)
	}
	return out
}

// The walker visits every Step of the tree, a fork's paths after the fork, in the order the fork
// lists them: its paths, then otherwise, then unknown.
func TestAllVisitsTheNestedStepsOfAForkAfterIt(t *testing.T) {
	visited := ids(automationstep.All(tree()))

	assert.Equal(t, []automationstep.ID{"a", "b", "x", "n", "y", "z", "c"}, visited)
}

// The Step after another is the next one of its path or, at the end of a path, the Step after the
// fork that holds the path, recursively; the last Step of the root has none.
func TestNextGoesOnAfterTheForkAtTheEndOfAPath(t *testing.T) {
	index := automationstep.NewIndex(tree())
	cases := map[automationstep.ID]automationstep.ID{
		"a": "b", "x": "n", "y": "c", "z": "c",
	}
	for from, want := range cases {
		next, ok := index.Next(from)
		require.True(t, ok, "from %s", from)
		assert.Equal(t, want, next.ID, "from %s", from)
	}
	_, ok := index.Next("c")
	assert.False(t, ok, "the last Step of the root has no next")
	_, ok = index.Next("elsewhere")
	assert.False(t, ok, "an id the version does not hold has no next")
}

// Taking a path reaches its first Step; an empty path goes on to the Step after its fork.
func TestEnteringAPathReachesItsFirstStepOrTheStepAfterTheFork(t *testing.T) {
	index := automationstep.NewIndex(tree())
	cases := map[automationstep.ID]automationstep.ID{
		"p1": "x", "p2": "c", "o": "z", "u": "c", "q": "y", "o2": "c",
	}
	for path, want := range cases {
		st, ok := index.Enter(path)
		require.True(t, ok, "path %s", path)
		assert.Equal(t, want, st.ID, "path %s", path)
	}
	_, ok := index.Enter("missing")
	assert.False(t, ok)
}

func TestTheIndexFindsANestedStep(t *testing.T) {
	index := automationstep.NewIndex(tree())

	st, ok := index.Step("y")
	_, missing := index.Step("missing")

	require.True(t, ok)
	assert.Equal(t, automationstep.Wait{Seconds: 6}, st.Payload)
	assert.False(t, missing)
}

// A rewrite reaches the Steps of every path, and the forks keep their paths' ids and Conditions;
// the tree given is left as it was.
func TestMapRewritesTheStepsOfEveryPath(t *testing.T) {
	held := tree()
	before, err := json.Marshal(held)
	require.NoError(t, err)

	out, err := automationstep.Map(held, func(pos int, st *automationstep.Step) (automationstep.Step, error) {
		if w, ok := st.Payload.(automationstep.Wait); ok {
			return automationstep.Step{ID: st.ID, Payload: automationstep.ApplyTag{Tag: strconv.Itoa(pos) + "/" + strconv.Itoa(w.Seconds)}}, nil
		}
		return *st, nil
	})
	require.NoError(t, err)
	after, err := json.Marshal(held)
	require.NoError(t, err)

	tags := tagsOf(out)
	assert.Equal(t, map[automationstep.ID]string{"a": "0/1", "x": "2/3", "y": "4/6", "z": "5/4", "c": "6/2"}, tags)
	b := out.Steps[1].Payload.(automationstep.Branch)
	assert.Equal(t, []automationstep.ID{"p1", "p2"}, []automationstep.ID{b.Paths[0].ID, b.Paths[1].ID})
	assert.Equal(t, vip(), b.Paths[0].Condition)
	assert.Equal(t, automationstep.ID("o"), b.Otherwise.ID)
	assert.Equal(t, automationstep.ID("u"), b.Unknown.ID)
	assert.JSONEq(t, string(before), string(after), "the tree mapped is left as it was")
}

// Every path of a fork gets an id, otherwise and unknown too, and a path's id may be no other's.
func TestIdentifyGivesTheNestedPathsIDs(t *testing.T) {
	branch := automationstep.New(automationstep.Branch{
		Paths:     []automationstep.BranchPath{{Condition: vip(), Steps: []automationstep.Step{automationstep.New(automationstep.Wait{Seconds: 1})}}},
		Otherwise: automationstep.Path{Steps: []automationstep.Step{}},
		Unknown:   &automationstep.Path{Steps: []automationstep.Step{}},
	})
	n := 0
	mint := func() automationstep.ID {
		n++
		return automationstep.ID("00000000-0000-4000-8000-" + strconv.Itoa(100000000000+n))
	}

	out, err := automationstep.Identify(automationstep.Root(branch), mint)

	require.NoError(t, err)
	b := out.Steps[0].Payload.(automationstep.Branch)
	assert.NotEmpty(t, b.Paths[0].ID)
	assert.NotEmpty(t, b.Paths[0].Steps[0].ID)
	assert.NotEmpty(t, b.Otherwise.ID)
	assert.NotEmpty(t, b.Unknown.ID)
	assert.Equal(t, 6, n, "the root, the Branch, its three paths and the Step of its first path")

	dup := automationstep.ID("00000000-0000-4000-8000-000000000001")
	twice := automationstep.New(automationstep.Branch{
		Paths:     []automationstep.BranchPath{{ID: dup, Condition: vip(), Steps: []automationstep.Step{}}},
		Otherwise: automationstep.Path{ID: dup, Steps: []automationstep.Step{}},
	})
	_, err = automationstep.Identify(automationstep.Root(twice), automationstep.NewID)
	require.ErrorIs(t, err, automationstep.ErrBadID)
	assert.Contains(t, err.Error(), "the path "+string(dup))
}

// An End finishes the path that holds it: a Step after it in a fork's path is found, and an End at
// the end of a path inside a fork lets the Steps after the fork be.
func TestAfterEndChecksEveryPathOnItsOwn(t *testing.T) {
	end := automationstep.Step{ID: "e", Payload: automationstep.End{}}
	inside := func(steps ...automationstep.Step) automationstep.Path {
		return automationstep.Root(automationstep.Step{ID: "b", Payload: automationstep.Branch{
			Paths:     []automationstep.BranchPath{{ID: "p", Condition: vip(), Steps: steps}},
			Otherwise: automationstep.Path{ID: "o", Steps: []automationstep.Step{}},
		}}, wait("after", 1))
	}

	after, found := automationstep.AfterEnd(inside(end, wait("t", 1)))
	_, endLast := automationstep.AfterEnd(inside(wait("t", 1), end))

	require.True(t, found)
	assert.Equal(t, automationstep.ID("t"), after.ID)
	assert.False(t, endLast, "an End last in its path lets the Step after the fork be")
}

// Every path of the tree is visited: the root, then each fork's paths, nested ones too.
func TestPathsVisitsEveryPathOfTheTree(t *testing.T) {
	var visited []automationstep.ID
	for p := range automationstep.Paths(tree()) {
		visited = append(visited, p.ID)
	}

	assert.Equal(t, []automationstep.ID{"root", "p1", "q", "o2", "p2", "o", "u"}, visited)
}

// A stored Branch is written with its paths, otherwise and unknown, and read back as it was; a
// field a Branch does not hold is refused.
func TestAStoredBranchRoundTrips(t *testing.T) {
	stored, err := json.Marshal(tree())
	require.NoError(t, err)

	var back automationstep.Path
	require.NoError(t, json.Unmarshal(stored, &back))
	again, err := json.Marshal(back)
	require.NoError(t, err)
	var unknown automationstep.Step
	refused := json.Unmarshal([]byte(`{"id":"b","type":"branch","paths":[],"otherwise":{"id":"o","steps":[]},"weight":3}`), &unknown)

	assert.JSONEq(t, string(stored), string(again))
	assert.Equal(t, ids(automationstep.All(tree())), ids(automationstep.All(back)))
	require.Error(t, refused)
}

// tagsOf is the Tag of each apply tag Step of the tree, by the Step's id.
func tagsOf(p automationstep.Path) map[automationstep.ID]string {
	tags := map[automationstep.ID]string{}
	for _, st := range automationstep.All(p) {
		if t, ok := st.Payload.(automationstep.ApplyTag); ok {
			tags[st.ID] = t.Tag
		}
	}
	return tags
}
