package drain_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/kernel/drain"
)

// A backlog of any length is removed whole, by as many full batches as it holds plus the short
// one that ends it.
func TestBatchesRemoveTheWholeBacklog(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		size := rapid.IntRange(1, 50).Draw(t, "size")
		backlog := rapid.IntRange(0, 500).Draw(t, "backlog")
		left, runs := backlog, 0
		removed, err := drain.Batches(size, func() (int, error) {
			runs++
			n := min(size, left)
			left -= n
			return n, nil
		})
		require.NoError(t, err)
		assert.Equal(t, backlog, removed)
		assert.Equal(t, backlog/size+1, runs)
	})
}

func TestBatchesStopAtAnErrorWithWhatWasRemovedBefore(t *testing.T) {
	failed := errors.New("delete failed")
	results := []error{nil, failed, nil}
	run := 0
	removed, err := drain.Batches(10, func() (int, error) {
		err := results[run]
		run++
		return 10, err
	})
	require.ErrorIs(t, err, failed)
	assert.Equal(t, 10, removed)
}
