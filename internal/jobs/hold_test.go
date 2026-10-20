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
	held := snoozeIfDeferrable(&HeldError{Reason: outbound.HoldSuspended})
	raced := snoozeIfDeferrable(fmt.Errorf("wrapped: %w", outbound.ErrInProgress))
	heldSnooze := snoozeOf(t, held)
	racedSnooze := snoozeOf(t, raced)
	assert.Equal(t, outbound.HoldBackoff, heldSnooze.Duration)
	assert.Equal(t, inProgressRetryDelay, racedSnooze.Duration)
}

// A Deferral is deferrable like a hold and snoozes by the delay the Outbound retry policy chose.
func TestDeferredErrorSnoozesByItsDelay(t *testing.T) {
	d := &DeferredError{Delay: 3500 * time.Millisecond}
	assert.True(t, isDeferrable(fmt.Errorf("wrapped: %w", d)))
	snooze := snoozeOf(t, snoozeIfDeferrable(d))
	assert.Equal(t, 3500*time.Millisecond, snooze.Duration)
}

func TestOtherErrorsPassThroughUnchanged(t *testing.T) {
	boom := errors.New("smtp unavailable")
	assert.Same(t, boom, snoozeIfDeferrable(boom))
	assert.False(t, isDeferrable(boom))
	assert.True(t, isDeferrable(&HeldError{Reason: outbound.HoldNoIntegration}))
	assert.True(t, isDeferrable(outbound.ErrInProgress))
}
