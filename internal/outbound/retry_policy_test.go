package outbound

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"
)

// The Deferral policy is tested with the jitter given: the exported Retry draws its own.

// A Deferral waits the limit's wait (floored at a second) scaled by the recipients still
// ahead of it, plus bounded jitter.
func TestADeferralRetriesAfterTheScaledWait(t *testing.T) {
	floored := deferralRetry(100*time.Millisecond, 0, 0)
	scaled := deferralRetry(500*time.Millisecond, 4, 0)
	jittered := deferralRetry(time.Second, 4, 1.0)
	perSecond := deferralRetry(70*time.Millisecond, 99, 0)
	capped := deferralRetry(12*time.Hour, 3, 0)

	assert.Equal(t, time.Second, floored.After, "floored at a second")
	assert.Equal(t, 2500*time.Millisecond, scaled.After, "each one ahead adds a token's wait")
	assert.Equal(t, 6*time.Second, jittered.After, "jitter adds at most 20 percent")
	assert.Equal(t, 7*time.Second, perSecond.After, "14 per second, 99 ahead")
	assert.Equal(t, time.Hour, capped.After, "capped so a raised limit is noticed")
}

// A long wait behind a large backlog waits the ceiling: the product once overflowed int64,
// wrapped negative and woke the job after the floor.
func TestADeferralBehindAHugeBacklogWaitsTheCeiling(t *testing.T) {
	retry := deferralRetry(48*time.Hour, 52_916, 0)
	assert.Equal(t, time.Hour, retry.After)
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

		retry := deferralRetry(wait, backlog, jitter)

		assert.GreaterOrEqual(t, retry.After, time.Second)
		assert.LessOrEqual(t, retry.After, time.Hour+12*time.Minute)
	})
}

// More recipients ahead, or more jitter, never make a job wake sooner.
func TestADeferralDelayNeverShrinksWithBacklogOrJitter(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		wait, backlog, jitter := drawDelay(t)
		moreBacklog := backlog + rapid.IntRange(0, 1000).Draw(t, "extraBacklog")
		moreJitter := jitter + (1-jitter)*rapid.Float64Range(0, 1).Draw(t, "extraJitter")

		base := deferralRetry(wait, backlog, jitter)
		busier := deferralRetry(wait, moreBacklog, jitter)
		jumpier := deferralRetry(wait, backlog, moreJitter)

		assert.GreaterOrEqual(t, busier.After, base.After)
		assert.GreaterOrEqual(t, jumpier.After, base.After)
	})
}
