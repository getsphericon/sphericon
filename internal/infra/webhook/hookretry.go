package webhook

import (
	"time"

	"github.com/jpillora/backoff"
)

// The retry schedule of an outbound call to a third-party hook (ADR 0039): a Webhook delivery,
// an HTTP step and a Rule's HTTP Action. A receiver that is down for a weekend still gets the
// event: retries run for HookRetryHorizon, not river's default of about four hours. The wait
// doubles from HookRetryFirstDelay up to HookRetryMaxDelay. Whether a failed call is retried at
// all is httpcall.Policy.Decide, built on these values, Retryable and RateLimited.
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

// hookBackoff is the doubling wait between hook attempts. Jitter is off: the library draws it
// from [Min, d], which would roughly halve the average wait, and HookMaxAttempts counts on the
// full waits to reach HookRetryHorizon. Only ForAttempt is called, which is pure and safe for
// concurrent use; the stateful Duration would share one counter between every job.
var hookBackoff = backoff.Backoff{Min: HookRetryFirstDelay, Max: HookRetryMaxDelay, Factor: 2}

// HookRetryDelay is the wait after the attempt-th failed attempt (1-based).
func HookRetryDelay(attempt int) time.Duration {
	return hookBackoff.ForAttempt(float64(attempt - 1))
}

// HookMaxAttempts is the MaxAttempts to insert a hook job with: the fewest attempts whose
// last one falls at or after HookRetryHorizon from the first.
func HookMaxAttempts() int {
	var elapsed time.Duration
	attempts := 1
	for ; elapsed < HookRetryHorizon; attempts++ {
		elapsed += HookRetryDelay(attempts)
	}
	return attempts
}

// HookNextRetry is when to retry after the failed attempt-th attempt, which started at
// attemptedAt. It is the body of a worker's NextRetry.
func HookNextRetry(attemptedAt time.Time, attempt int) time.Time {
	return attemptedAt.Add(HookRetryDelay(attempt))
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
