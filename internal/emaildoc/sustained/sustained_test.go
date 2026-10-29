package sustained_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/emaildoc/sustained"
)

// observeAll feeds the polls to a counter of need and returns whether it fired on each.
func observeAll(need int, polls []bool) []bool {
	c := sustained.New(need)
	return lo.Map(polls, func(over bool, _ int) bool { return c.Observe(over) })
}

// runModel is the specification: a poll fires when it and the need-1 polls before it all read
// over the limit.
func runModel(need int, polls []bool) []bool {
	fired := make([]bool, len(polls))
	run := 0
	for i, over := range polls {
		if over {
			run++
		} else {
			run = 0
		}
		fired[i] = run >= need
	}
	return fired
}

// The heap counter is process-wide, so a burst of allocations by something else that does not
// last must not stop a render that is not its cause: only polls over the limit in a row count.
func TestOnlyAnUnbrokenRunOfPollsOverTheLimitFires(t *testing.T) {
	burst := []bool{true, true, false}
	polls := append(lo.Flatten(lo.Times(10, func(int) []bool { return burst })), true, true, true)

	fired := observeAll(3, polls)

	require.Len(t, fired, len(polls))
	assert.Equal(t, append(make([]bool, len(polls)-1), true), fired,
		"two polls over the limit are a burst; only the closing run of three fires")
}

// Whatever the polls, the counter fires exactly when the last `need` of them all read over the
// limit: a model of the run is the whole specification.
func TestFiresOnlyAfterAnUnbrokenRunOverTheLimit(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		need := rapid.IntRange(1, 12).Draw(t, "need")
		polls := rapid.SliceOfN(rapid.Bool(), 0, 80).Draw(t, "polls")

		assert.Equal(t, runModel(need, polls), observeAll(need, polls))
	})
}
