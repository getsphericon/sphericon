package emaildoc_test

import (
	"context"
	"runtime"
	"sync"
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
// last must not stop a render that is not its cause.
func TestAnotherGoroutinesAllocationBurstDoesNotStopARender(t *testing.T) {
	newsletter := readDoc(t, "newsletter.json")
	require.NoError(t, shared().Validate(context.Background(), newsletter))

	stop := make(chan struct{})
	var bursts sync.WaitGroup
	bursts.Add(1)
	go func() {
		defer bursts.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			burst := make([]byte, 160<<20)
			burst[0] = 1
			time.Sleep(5 * time.Millisecond)
			runtime.KeepAlive(burst)
			runtime.GC()
			time.Sleep(150 * time.Millisecond)
		}
	}()
	defer func() {
		close(stop)
		bursts.Wait()
	}()

	for range 15 {
		_, err := shared().Render(context.Background(), newsletter, emaildoc.WithMemoryGrowth(96<<20))
		require.NoError(t, err)
	}
}
