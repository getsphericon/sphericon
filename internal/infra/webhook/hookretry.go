package webhook

import (
	"time"

	"github.com/jpillora/backoff"
)

// The retry schedule of an outbound call to a third-party hook (ADR 0039): a Webhook delivery,
// an HTTP step and a Rule's HTTP Action. A receiver that is down for a weekend still gets the
// event: retries run for HookRetryHorizon, not river's default of about four hours. The wait
// doubles from HookRetryFirstDelay up to HookRetryMaxDelay, and each is spread by ±10%
// (HookRetryWait). Whether a failed call is retried at all is httpcall.Policy.Decide, built on
// these values, Retryable and RateLimited.
const (
	HookRetryHorizon    = 72 * time.Hour
	HookRetryFirstDelay = 30 * time.Second
	HookRetryMaxDelay   = 6 * time.Hour
)

// A 429 snoozes the job for the receiver's Retry-After, never an attempt: HookSnoozeDefault
// when the header is absent or unreadable, at most HookSnoozeMax.
const (
	HookSnoozeDefault = time.Minute
	HookSnoozeMax     = 6 * time.Hour
)

// HookRetryDelay is the delay computed for the attempt-th failed attempt (1-based), before the
// jitter HookRetryWait spreads it by. The library's own jitter is off: it draws from [Min, d],
// which would roughly halve the average wait.
func HookRetryDelay(attempt int) time.Duration {
	b := backoff.Backoff{Min: HookRetryFirstDelay, Max: HookRetryMaxDelay, Factor: 2}
	return b.ForAttempt(float64(attempt - 1))
}

// hookRetryBounds is the shortest and the longest wait after the attempt-th failed attempt:
// HookRetryDelay less and plus a tenth of it.
func hookRetryBounds(attempt int) (lower, upper time.Duration) {
	delay := HookRetryDelay(attempt)
	spread := delay / 10
	return delay - spread, delay + spread
}

// HookRetryWait is the wait after the attempt-th failed attempt for the draw u in [0, 1]: spread
// uniformly across hookRetryBounds, so that u = 0.5 waits HookRetryDelay itself. The jitter keeps
// deliveries that failed together against one receiver from all retrying at the same instant
// when it comes back, the shape of river's default policy.
func HookRetryWait(attempt int, u float64) time.Duration {
	lower, upper := hookRetryBounds(attempt)
	return lower + time.Duration(u*float64(upper-lower))
}

// HookMaxAttempts is the MaxAttempts to insert a hook job with: the fewest attempts whose
// last one falls at or after HookRetryHorizon from the first, counted on the lower bound of
// every wait, so that it does whatever the jitter draws.
func HookMaxAttempts() int {
	var elapsed time.Duration
	attempts := 1
	for ; elapsed < HookRetryHorizon; attempts++ {
		lower, _ := hookRetryBounds(attempts)
		elapsed += lower
	}
	return attempts
}

// HookJitter is the source of a hook retry's jitter: each call returns a draw in [0, 1), as
// rand.Float64 does, which the river client's workers are built with. A test passes a fixed draw.
type HookJitter func() float64

// HookNextRetry is when to retry after the failed attempt-th attempt, which started at
// attemptedAt, for a draw of jitter. It is the body of a worker's NextRetry.
func HookNextRetry(attemptedAt time.Time, attempt int, jitter HookJitter) time.Time {
	wait := HookRetryWait(attempt, jitter())
	return attemptedAt.Add(wait)
}

// HookSnooze is how long to snooze a job whose receiver answered 429 with the given
// Retry-After value.
func HookSnooze(retryAfter string, now time.Time) time.Duration {
	wait, ok := ParseRetryAfter(retryAfter, now)
	if !ok || wait <= 0 {
		return HookSnoozeDefault
	}
	return min(wait, HookSnoozeMax)
}
