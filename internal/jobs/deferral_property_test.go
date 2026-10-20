package jobs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"
)

func drawDelay(t *rapid.T) (wait time.Duration, backlog int, jitter float64) {
	wait = time.Duration(rapid.Int64Range(0, int64(48*time.Hour)).Draw(t, "wait"))
	backlog = rapid.IntRange(0, 100_000).Draw(t, "backlog")
	jitter = rapid.Float64Range(0, 1).Draw(t, "jitter")
	return wait, backlog, jitter
}

// A Deferral never wakes sooner than the floor and never later than the ceiling plus its jitter.
func TestDeferralDelayStaysBetweenTheFloorAndTheJitteredCeiling(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		wait, backlog, jitter := drawDelay(t)

		got := deferralDelay(wait, backlog, jitter)

		if got < deferralFloor {
			t.Fatalf("%v is under the floor %v", got, deferralFloor)
		}
		ceiling := deferralCeiling + time.Duration(float64(deferralCeiling)*deferralJitter)
		if got > ceiling {
			t.Fatalf("%v is over the jittered ceiling %v", got, ceiling)
		}
	})
}

// More recipients ahead, or more jitter, never make a job wake sooner.
func TestDeferralDelayNeverShrinksWithBacklogOrJitter(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		wait, backlog, jitter := drawDelay(t)
		moreBacklog := backlog + rapid.IntRange(0, 1000).Draw(t, "extraBacklog")
		moreJitter := jitter + (1-jitter)*rapid.Float64Range(0, 1).Draw(t, "extraJitter")

		base := deferralDelay(wait, backlog, jitter)

		if got := deferralDelay(wait, moreBacklog, jitter); got < base {
			t.Fatalf("backlog %d -> %d shrank the delay: %v -> %v", backlog, moreBacklog, base, got)
		}
		if got := deferralDelay(wait, backlog, moreJitter); got < base {
			t.Fatalf("jitter %v -> %v shrank the delay: %v -> %v", jitter, moreJitter, base, got)
		}
	})
}

// A long wait behind a large backlog waits the ceiling: the product of the two once overflowed
// int64, wrapped negative and woke the job after the floor.
func TestDeferralDelayOfAHugeBacklogIsTheCeiling(t *testing.T) {
	got := deferralDelay(48*time.Hour, 52_916, 0)

	assert.Equal(t, deferralCeiling, got)
}
