package webhook_test

import (
	"math/rand/v2"
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
		got := webhook.HookRetryDelay(i + 1)
		assert.Equal(t, delay, got, "attempt %d", i+1)
	}
}

// A retry waits the delay of its attempt spread by up to 10% either way: the draw 0 waits 10% less,
// 0.5 the delay itself, and the draw 1 10% more.
func TestHookRetryWaitSpreadsTheDelayByTenPercent(t *testing.T) {
	cases := []struct {
		attempt int
		u       float64
		want    time.Duration
	}{
		{1, 0, 27 * time.Second},
		{1, 0.5, 30 * time.Second},
		{1, 1, 33 * time.Second},
		{11, 0, 324 * time.Minute},
		{11, 0.5, 6 * time.Hour},
		{11, 1, 396 * time.Minute},
	}
	for _, c := range cases {
		got := webhook.HookRetryWait(c.attempt, c.u)
		assert.Equal(t, c.want, got, "attempt %d, draw %v", c.attempt, c.u)
	}
}

// Whatever the attempt and the draw, a retry waits within 10% of the delay of its attempt.
func TestHookRetryWaitStaysWithinTenPercentOfTheDelay(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		attempt := rapid.IntRange(1, 1_000_000).Draw(rt, "attempt")
		u := rapid.Float64Range(0, 1).Draw(rt, "u")

		wait, delay := webhook.HookRetryWait(attempt, u), doublingDelay(attempt)
		outside := 10*wait < 9*delay || 10*wait > 11*delay

		if outside {
			rt.Fatalf("attempt %d, draw %v: wait %v, outside 10%% of %v", attempt, u, wait, delay)
		}
	})
}

// The next retry is the attempt's wait for the draw of the jitter source it is given.
func TestHookNextRetryDrawsFromItsJitterSource(t *testing.T) {
	at := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	lowest := func() float64 { return 0 }
	middle := func() float64 { return 0.5 }

	first, fourth := webhook.HookNextRetry(at, 1, lowest), webhook.HookNextRetry(at, 4, middle)

	assert.Equal(t, time.Date(2026, 10, 18, 9, 0, 27, 0, time.UTC), first)
	assert.Equal(t, time.Date(2026, 10, 18, 9, 4, 0, 0, time.UTC), fourth)
}

// With the real source, every retry up to the last attempt falls within 10% of its delay.
func TestHookNextRetryWithTheRealSourceStaysWithinTheBounds(t *testing.T) {
	at := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	for attempt := 1; attempt <= webhook.HookMaxAttempts(); attempt++ {
		next := webhook.HookNextRetry(at, attempt, rand.Float64)
		wait, delay := next.Sub(at), doublingDelay(attempt)
		lower, upper := delay*9/10, delay*11/10
		assert.GreaterOrEqual(t, wait, lower, "attempt %d", attempt)
		assert.LessOrEqual(t, wait, upper, "attempt %d", attempt)
	}
}

// 23 attempts cover the 72-hour horizon however the retries are drawn: at their shortest, ten
// doubling waits of about 7 h 40 min in all, then twelve waits of 5 h 24 min.
func TestHookMaxAttemptsCoverTheHorizonAtTheShortestWaits(t *testing.T) {
	attempts := webhook.HookMaxAttempts()
	last, beforeLast := shortestElapsed(attempts), shortestElapsed(attempts-1)

	assert.Equal(t, 23, attempts)
	assert.GreaterOrEqual(t, last, 72*time.Hour)
	assert.Less(t, beforeLast, 72*time.Hour, "one attempt fewer falls short")
}

// shortestElapsed is how long after the first attempt the last of attempts falls when every
// retry waits 90% of the doubling schedule.
func shortestElapsed(attempts int) time.Duration {
	var elapsed time.Duration
	for attempt := 1; attempt < attempts; attempt++ {
		elapsed += doublingDelay(attempt) * 9 / 10
	}
	return elapsed
}

// Every attempt up to the last one and five past it waits what the doubling schedule gives.
func TestHookRetryDelayAgreesWithTheDoublingScheduleUpToTheLastAttempt(t *testing.T) {
	last := webhook.HookMaxAttempts() + 5
	for attempt := 1; attempt <= last; attempt++ {
		got, want := webhook.HookRetryDelay(attempt), doublingDelay(attempt)
		assert.Equal(t, want, got, "attempt %d", attempt)
	}
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

// doublingDelay is the schedule as a plain loop: 30 s, doubled per attempt, capped at 6 hours.
func doublingDelay(attempt int) time.Duration {
	delay := 30 * time.Second
	for i := 1; i < attempt && delay < 6*time.Hour; i++ {
		delay *= 2
	}
	return min(delay, 6*time.Hour)
}
