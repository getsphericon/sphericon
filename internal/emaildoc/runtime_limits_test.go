package emaildoc_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/emaildoc"
)

// The heap counter is process-wide, so a burst of allocations by something else that does not
// last must not stop a render that is not its cause: only polls over the limit in a row count.
func TestOnlyAnUnbrokenRunOfPollsOverTheLimitStopsARender(t *testing.T) {
	s := emaildoc.NewSustained(3)

	for range 10 {
		require.False(t, s.Observe(true))
		require.False(t, s.Observe(true), "two polls over the limit are a burst, not growth")
		require.False(t, s.Observe(false), "a poll back under the limit starts the count again")
	}

	require.False(t, s.Observe(true))
	require.False(t, s.Observe(true))
	require.True(t, s.Observe(true), "the limit stayed over for the whole run")
}
