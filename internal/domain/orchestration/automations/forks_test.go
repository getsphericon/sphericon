package automations_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// forkID is the id of the fork a refusal names.
const forkID = "7c1e2a4b-3d5f-4a6b-8c9d-0e1f2a3b4c5d"

// branchOf is a Branch with n listed paths, each with no Condition: the count of paths is checked
// before any Condition.
func branchOf(n int) automationstep.Branch {
	return automationstep.Branch{Paths: make([]automationstep.BranchPath, n)}
}

// splitOf is a Split with a path of each weight.
func splitOf(weights ...int) automationstep.Split {
	paths := make([]automationstep.SplitPath, len(weights))
	for i, w := range weights {
		paths[i] = automationstep.SplitPath{Weight: w}
	}
	return automationstep.Split{Paths: paths}
}

// A fork outside the contract's bounds (the listed paths of a Branch, the paths of a Split and the
// least weight) is refused by the module too, naming the fork by its id and kind (ADR 0122), for a
// caller that is no HTTP surface.
func TestAForkOutsideTheContractsBoundsIsRefusedNamingIt(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*automations.Module](env)
	for name, c := range map[string]struct {
		fork  automationstep.Payload
		names string
	}{
		"a branch with only otherwise": {branchOf(0), "step " + forkID + " (branch): a branch has 1 to 9 paths besides otherwise, this one has 0"},
		"a branch of ten listed paths": {branchOf(10), "step " + forkID + " (branch): a branch has 1 to 9 paths besides otherwise, this one has 10"},
		"a split of one path":          {splitOf(100), "step " + forkID + " (split): a split has 2 to 10 paths, this one has 1"},
		"a split of eleven paths":      {splitOf(10, 10, 10, 10, 10, 10, 10, 10, 10, 5, 5), "step " + forkID + " (split): a split has 2 to 10 paths, this one has 11"},
		"a weight of 0":                {splitOf(100, 0), "step " + forkID + " (split): path 2: a weight is at least 1, this one is 0"},
	} {
		t.Run(name, func(t *testing.T) {
			steps := automationstep.Root(automationstep.Step{ID: forkID, Payload: c.fork})

			_, err := m.Create(t.Context(), env.DB.Scoped(fixtures.AcmeID), automations.CreateInput{Name: "x", Trigger: eventmatcher.Matcher{Action: "y"}, Steps: &steps})

			require.ErrorIs(t, err, automations.ErrInvalidStep)
			require.ErrorContains(t, err, c.names)
		})
	}
}
