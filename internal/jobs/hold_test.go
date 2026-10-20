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
	assert.Equal(t, outbound.HoldBackoff, snoozeOf(t, snoozeIfDeferrable(&HeldError{Reason: outbound.HoldSuspended})).Duration)
	assert.Equal(t, inProgressRetryDelay,
		snoozeOf(t, snoozeIfDeferrable(fmt.Errorf("wrapped: %w", outbound.ErrInProgress))).Duration)
}

// A Deferral is deferrable like a hold and snoozes by the Outbound retry policy.
func TestDeferredErrorSnoozesByThePolicy(t *testing.T) {
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
