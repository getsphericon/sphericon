package outbound_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/outbound"
)

func deferral(wait time.Duration) outbound.Result {
	return outbound.Result{Outcome: outbound.Deferral, Wait: wait}
}

// Only a Hold and a Deferral want a retry: every other Outcome is final.
func TestOnlyAHoldOrADeferralWantsARetry(t *testing.T) {
	for _, o := range []outbound.Outcome{outbound.Sent, outbound.Skipped, outbound.Failed, outbound.Refused} {
		assert.False(t, outbound.Result{Outcome: o}.Retry(0, 0).Wanted, o)
	}
	assert.True(t, outbound.Result{Outcome: outbound.Held}.Retry(0, 0).Wanted)
	assert.True(t, deferral(time.Second).Retry(0, 0).Wanted)
}

// A Hold retries after a fixed backoff whatever the source, so a Broadcast recipient and an
// Automation run held by the same freeze come back on the same schedule.
func TestAHoldRetriesAfterTheHoldBackoffForEverySource(t *testing.T) {
	for _, reason := range []string{outbound.HoldSuspended, outbound.HoldStaff, outbound.HoldNoIntegration} {
		got := outbound.Result{Outcome: outbound.Held, Reason: reason}.Retry(0, 0.7)
		assert.Equal(t, 15*time.Minute, got.After, reason)
	}
}

// A Deferral waits the limit's wait (floored at a second) scaled by the recipients still
// ahead of it, plus bounded jitter.
func TestADeferralRetriesAfterTheScaledWait(t *testing.T) {
	assert.Equal(t, time.Second, deferral(100*time.Millisecond).Retry(0, 0).After, "floored at a second")
	assert.Equal(t, 2500*time.Millisecond, deferral(500*time.Millisecond).Retry(4, 0).After, "each one ahead adds a token's wait")
	assert.Equal(t, 6*time.Second, deferral(time.Second).Retry(4, 1.0).After, "jitter adds at most 20 percent")
	assert.Equal(t, 7*time.Second, deferral(70*time.Millisecond).Retry(99, 0).After, "14 per second, 99 ahead")
	assert.Equal(t, time.Hour, deferral(12*time.Hour).Retry(3, 0).After, "capped so a raised limit is noticed")
}

// A long wait behind a large backlog waits the ceiling: the product once overflowed int64,
// wrapped negative and woke the job after the floor.
func TestADeferralBehindAHugeBacklogWaitsTheCeiling(t *testing.T) {
	assert.Equal(t, time.Hour, deferral(48*time.Hour).Retry(52_916, 0).After)
}

func drawDelay(t *rapid.T) (wait time.Duration, backlog int, jitter float64) {
	wait = time.Duration(rapid.Int64Range(0, int64(48*time.Hour)).Draw(t, "wait"))
	backlog = rapid.IntRange(0, 100_000).Draw(t, "backlog")
	jitter = rapid.Float64Range(0, 1).Draw(t, "jitter")
	return wait, backlog, jitter
}

// A Deferral never wakes sooner than the floor and never later than the ceiling plus jitter.
func TestADeferralDelayStaysWithinTheBounds(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		wait, backlog, jitter := drawDelay(t)

		got := deferral(wait).Retry(backlog, jitter).After

		if got < time.Second || got > time.Hour+12*time.Minute {
			t.Fatalf("%v is outside [1s, 1h12m]", got)
		}
	})
}

// More recipients ahead, or more jitter, never make a job wake sooner.
func TestADeferralDelayNeverShrinksWithBacklogOrJitter(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		wait, backlog, jitter := drawDelay(t)
		moreBacklog := backlog + rapid.IntRange(0, 1000).Draw(t, "extraBacklog")
		moreJitter := jitter + (1-jitter)*rapid.Float64Range(0, 1).Draw(t, "extraJitter")

		base := deferral(wait).Retry(backlog, jitter).After

		if got := deferral(wait).Retry(moreBacklog, jitter).After; got < base {
			t.Fatalf("backlog %d -> %d shrank the delay: %v -> %v", backlog, moreBacklog, base, got)
		}
		if got := deferral(wait).Retry(backlog, moreJitter).After; got < base {
			t.Fatalf("jitter %v -> %v shrank the delay: %v -> %v", jitter, moreJitter, base, got)
		}
	})
}
