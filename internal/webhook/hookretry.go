package webhook

import "time"

// The retry policy of an outbound call to a third-party hook (ADR 0039): a Webhook delivery
// and, through the same functions, an HTTP step. A receiver that is down for a weekend
// still gets the event: retries run for HookRetryHorizon, not river's default of about
// four hours. The wait doubles from HookRetryFirstDelay up to HookRetryMaxDelay.
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

// HookRetryDelay is the wait after the attempt-th failed attempt (1-based).
func HookRetryDelay(attempt int) time.Duration {
	delay := HookRetryFirstDelay
	for i := 1; i < attempt && delay < HookRetryMaxDelay; i++ {
		delay *= 2
	}
	return min(delay, HookRetryMaxDelay)
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
