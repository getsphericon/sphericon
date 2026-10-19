package emaildoc_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/emaildoc"
)

// A call whose context ends while the runtime is still starting returns at once instead of
// waiting out the start, and the runtime that finishes starting afterwards serves the next call.
func TestCallEndsWhileTheRuntimeIsStartingAndTheNextOneStillRenders(t *testing.T) {
	r := emaildoc.New()
	newsletter := readDoc(t, "newsletter.json")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	began := time.Now()
	_, err := r.Render(ctx, newsletter)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(began), 500*time.Millisecond, "the call does not wait for the start")

	got, err := r.Render(context.Background(), newsletter)
	require.NoError(t, err)
	require.NotEmpty(t, got.HTML)
	require.True(t, r.Started())
}

// The start has a limit of its own.
func TestTheStartTimeoutBoundsTheRuntimeStart(t *testing.T) {
	limits := emaildoc.DefaultLimits()
	limits.StartTimeout = 20 * time.Millisecond
	r := emaildoc.New(emaildoc.WithLimits(limits))
	newsletter := readDoc(t, "newsletter.json")

	began := time.Now()
	_, err := r.Render(context.Background(), newsletter)
	require.ErrorIs(t, err, emaildoc.ErrTimeout)
	require.Less(t, time.Since(began), 500*time.Millisecond)

	_, err = r.Render(context.Background(), newsletter)
	require.NoError(t, err)
}

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
