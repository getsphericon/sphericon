package httpcall

import (
	"time"

	"github.com/getsphericon/sphericon/internal/infra/webhook"
)

// Policy is how the owner of an outbound call retries one that failed (ADR 0039). Every outbound
// call to a third party decides through Policy.Decide, and its owner keeps only its own side
// effects (a failure recorded, a job snoozed or cancelled).
//
// There are exactly two, Subscription and OneOff: its field is unexported, so no other value can
// be built, and its zero value is OneOff.
type Policy struct {
	retriesAny bool
}

var (
	// Subscription retries any failure: a Webhook delivery. Its endpoint is a subscription whose
	// owner fixes their side while the retries run, and which disables itself by its own rule (a
	// 410, or persistent failure), so even an answer another attempt would not change is retried.
	Subscription = Policy{retriesAny: true}
	// OneOff retries only what another attempt can fix (webhook.Retryable): an HTTP step and a
	// Rule's HTTP Action, a call made once for one Contact. Any other failure gives up at once.
	OneOff = Policy{}
)

// Outcome is what the owner of a failed call does next.
type Outcome string

const (
	// Retry makes another attempt on the call's ordinary schedule.
	Retry Outcome = "retry"
	// Wait asks again after Decision.Wait without spending an attempt: the receiver answered 429.
	Wait Outcome = "wait"
	// GiveUp ends the call as failed, for Decision.Reason.
	GiveUp Outcome = "give_up"
)

// Reason is why a call is given up.
type Reason string

const (
	// NotRetryable: another attempt would get the same answer (a blocked address, a 4xx other
	// than a 408 or a 429). Only a one-off call gives up for it.
	NotRetryable Reason = "not_retryable"
	// AttemptsExhausted: the attempt that failed was the call's last.
	AttemptsExhausted Reason = "attempts_exhausted"
	// RateLimitedPastHorizon: the receiver still answers 429 webhook.HookRetryHorizon after the
	// call was first queued.
	RateLimitedPastHorizon Reason = "rate_limited_past_horizon"
)

// Decision is what to do after a failed attempt. Wait is set only for the Wait outcome, Reason
// only for GiveUp.
type Decision struct {
	Outcome Outcome
	Wait    time.Duration
	Reason  Reason
}

// Decide is what to do after attempt failed with sendErr, the error of webhook.Post or
// webhook.Send, at now, read once the receiver has answered. A success is not its business: the
// owner handles a nil sendErr before. A 429 is neither a success nor a failure: it waits as long
// as the receiver asked (webhook.HookSnooze), spending no attempt, until HookRetryHorizon has
// passed since the call was queued. Any other failure gives up when the policy does not retry it
// or when it was the last attempt, and retries otherwise.
func (p Policy) Decide(sendErr error, attempt Attempt, now time.Time) Decision {
	if limited := webhook.RateLimited(sendErr); limited != nil {
		if now.Sub(attempt.CreatedAt) >= webhook.HookRetryHorizon {
			return Decision{Outcome: GiveUp, Reason: RateLimitedPastHorizon}
		}
		return Decision{Outcome: Wait, Wait: webhook.HookSnooze(limited.RetryAfter, now)}
	}
	if !p.retriesAny && !webhook.Retryable(sendErr) {
		return Decision{Outcome: GiveUp, Reason: NotRetryable}
	}
	if attempt.Number >= attempt.Limit {
		return Decision{Outcome: GiveUp, Reason: AttemptsExhausted}
	}
	return Decision{Outcome: Retry}
}
