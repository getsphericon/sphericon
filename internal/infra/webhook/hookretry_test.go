package webhook_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/infra/webhook"
)

// The wait after each failed attempt doubles from 30 s until it reaches the 6-hour cap at the
// 11th attempt.
func TestHookRetryDelayDoublesFromThirtySecondsToTheCap(t *testing.T) {
	want := []time.Duration{
		30 * time.Second,
		time.Minute,
		2 * time.Minute,
		4 * time.Minute,
		8 * time.Minute,
		16 * time.Minute,
		32 * time.Minute,
		64 * time.Minute,
		128 * time.Minute,
		256 * time.Minute,
		6 * time.Hour,
	}
	for i, delay := range want {
		assert.Equal(t, delay, webhook.HookRetryDelay(i+1), "attempt %d", i+1)
	}
}

// Every attempt past the 11th waits the cap, up to the last attempt and beyond it.
func TestHookRetryDelayStaysAtTheCap(t *testing.T) {
	for attempt := 11; attempt <= webhook.HookMaxAttempts()+5; attempt++ {
		assert.Equal(t, 6*time.Hour, webhook.HookRetryDelay(attempt), "attempt %d", attempt)
	}
}

// 22 attempts cover the 72-hour horizon: ten doubling waits of 8.5 hours in all, then eleven
// waits of 6 hours.
func TestHookMaxAttemptsCoverTheHorizon(t *testing.T) {
	assert.Equal(t, 22, webhook.HookMaxAttempts())
}

func TestHookRetryDelayAgreesWithTheDoublingSchedule(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		attempt := rapid.IntRange(1, 1_000_000).Draw(rt, "attempt")

		got, want := webhook.HookRetryDelay(attempt), doublingDelay(attempt)

		if got != want {
			rt.Fatalf("attempt %d: delay %v, want %v", attempt, got, want)
		}
	})
}

// doublingDelay is the schedule as the hand-written loop computed it before the backoff
// library: 30 s, doubled per attempt, capped at 6 hours.
func doublingDelay(attempt int) time.Duration {
	delay := 30 * time.Second
	for i := 1; i < attempt && delay < 6*time.Hour; i++ {
		delay *= 2
	}
	return min(delay, 6*time.Hour)
}
