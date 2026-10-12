package jobs

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/outbound"
)

func snoozeOf(t *testing.T, err error) *rivertype.JobSnoozeError {
	t.Helper()
	var s *rivertype.JobSnoozeError
	require.ErrorAs(t, err, &s)
	return s
}

// A hold and a live claim are both "try again later", never "spend an attempt": a
// long hold must not exhaust a recipient's retry budget, and an attempt that merely
// raced another must not be marked failed.
func TestDeferrableErrorsBecomeSnoozes(t *testing.T) {
	assert.Equal(t, holdRetryDelay, snoozeOf(t, snoozeIfDeferrable(&HeldError{Reason: outbound.HoldSuspended})).Duration)
	assert.Equal(t, inProgressRetryDelay,
		snoozeOf(t, snoozeIfDeferrable(fmt.Errorf("wrapped: %w", outbound.ErrInProgress))).Duration)
}

// A Deferral snoozes for the wait (floored at a second) scaled by the recipients still
// ahead of the job, plus bounded jitter; it is deferrable like a hold.
func TestDeferralDelayScalesWithBacklogAndIsBounded(t *testing.T) {
	assert.Equal(t, time.Second, deferralDelay(100*time.Millisecond, 0, 0), "floored at a second")
	assert.Equal(t, 2500*time.Millisecond, deferralDelay(500*time.Millisecond, 4, 0), "each recipient ahead adds one token's wait")
	assert.Equal(t, 6*time.Second, deferralDelay(time.Second, 4, 1.0), "jitter adds at most 20 percent")
	assert.Equal(t, 7*time.Second, deferralDelay(70*time.Millisecond, 99, 0), "14 per second, 99 ahead")
	assert.Equal(t, time.Hour, deferralDelay(12*time.Hour, 3, 0), "capped so a raised limit is noticed")

	d := &DeferredError{Wait: time.Second, Backlog: 2}
	assert.True(t, isDeferrable(fmt.Errorf("wrapped: %w", d)))
	got := snoozeOf(t, snoozeIfDeferrable(d)).Duration
	assert.GreaterOrEqual(t, got, 3*time.Second)
	assert.LessOrEqual(t, got, 3600*time.Millisecond)
}

func TestOtherErrorsPassThroughUnchanged(t *testing.T) {
	boom := errors.New("smtp unavailable")
	assert.Same(t, boom, snoozeIfDeferrable(boom))
	assert.False(t, isDeferrable(boom))
	assert.True(t, isDeferrable(&HeldError{Reason: outbound.HoldNoIntegration}))
	assert.True(t, isDeferrable(outbound.ErrInProgress))
}
