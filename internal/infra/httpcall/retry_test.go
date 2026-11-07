package httpcall_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
)

var queuedAt = time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)

func status(code int, retryAfter string) error {
	return &webhook.DeliveryError{Kind: webhook.KindHTTPStatus, Status: code, RetryAfter: retryAfter}
}

var (
	retry = httpcall.Decision{Outcome: httpcall.Retry}
	// exhausted, notRetryable and pastHorizon are the three ways a call is given up.
	exhausted    = httpcall.Decision{Outcome: httpcall.GiveUp, Reason: httpcall.AttemptsExhausted}
	notRetryable = httpcall.Decision{Outcome: httpcall.GiveUp, Reason: httpcall.NotRetryable}
	pastHorizon  = httpcall.Decision{Outcome: httpcall.GiveUp, Reason: httpcall.RateLimitedPastHorizon}
)

func wait(d time.Duration) httpcall.Decision {
	return httpcall.Decision{Outcome: httpcall.Wait, Wait: d}
}

func TestEachPolicyDecidesEveryFailureWithAttemptsLeftAndOnTheLastAttempt(t *testing.T) {
	cases := map[string]struct {
		err error
		// age is how long before the attempt failed the call was queued.
		age time.Duration
		// The decision of a one-off call and of a subscription, on attempt 1 of 3 and on attempt 3 of 3.
		oneOffLeft, oneOffLast, subscriptionLeft, subscriptionLast httpcall.Decision
	}{
		"timeout": {
			err:        &webhook.DeliveryError{Kind: webhook.KindTimeout},
			oneOffLeft: retry, oneOffLast: exhausted, subscriptionLeft: retry, subscriptionLast: exhausted,
		},
		"transport error": {
			err:        &webhook.DeliveryError{Kind: webhook.KindNetwork},
			oneOffLeft: retry, oneOffLast: exhausted, subscriptionLeft: retry, subscriptionLast: exhausted,
		},
		"an error the client did not classify": {
			err:        errors.New("connection reset"),
			oneOffLeft: retry, oneOffLast: exhausted, subscriptionLeft: retry, subscriptionLast: exhausted,
		},
		"5xx": {
			err:        status(http.StatusServiceUnavailable, ""),
			oneOffLeft: retry, oneOffLast: exhausted, subscriptionLeft: retry, subscriptionLast: exhausted,
		},
		"408": {
			err:        status(http.StatusRequestTimeout, ""),
			oneOffLeft: retry, oneOffLast: exhausted, subscriptionLeft: retry, subscriptionLast: exhausted,
		},
		"429 with Retry-After": {
			err:        status(http.StatusTooManyRequests, "120"),
			oneOffLeft: wait(120 * time.Second), oneOffLast: wait(120 * time.Second),
			subscriptionLeft: wait(120 * time.Second), subscriptionLast: wait(120 * time.Second),
		},
		"429 without Retry-After": {
			err:        status(http.StatusTooManyRequests, ""),
			oneOffLeft: wait(webhook.HookSnoozeDefault), oneOffLast: wait(webhook.HookSnoozeDefault),
			subscriptionLeft: wait(webhook.HookSnoozeDefault), subscriptionLast: wait(webhook.HookSnoozeDefault),
		},
		"429 just inside the horizon": {
			err: status(http.StatusTooManyRequests, "120"), age: webhook.HookRetryHorizon - time.Second,
			oneOffLeft: wait(120 * time.Second), oneOffLast: wait(120 * time.Second),
			subscriptionLeft: wait(120 * time.Second), subscriptionLast: wait(120 * time.Second),
		},
		"429 past the horizon": {
			err: status(http.StatusTooManyRequests, "120"), age: webhook.HookRetryHorizon,
			oneOffLeft: pastHorizon, oneOffLast: pastHorizon, subscriptionLeft: pastHorizon, subscriptionLast: pastHorizon,
		},
		"400": {
			err:        status(http.StatusBadRequest, ""),
			oneOffLeft: notRetryable, oneOffLast: notRetryable, subscriptionLeft: retry, subscriptionLast: exhausted,
		},
		"401": {
			err:        status(http.StatusUnauthorized, ""),
			oneOffLeft: notRetryable, oneOffLast: notRetryable, subscriptionLeft: retry, subscriptionLast: exhausted,
		},
		"404": {
			err:        status(http.StatusNotFound, ""),
			oneOffLeft: notRetryable, oneOffLast: notRetryable, subscriptionLeft: retry, subscriptionLast: exhausted,
		},
		"410": {
			err:        status(http.StatusGone, ""),
			oneOffLeft: notRetryable, oneOffLast: notRetryable, subscriptionLeft: retry, subscriptionLast: exhausted,
		},
		"blocked address": {
			err:        &webhook.DeliveryError{Kind: webhook.KindBlockedAddress},
			oneOffLeft: notRetryable, oneOffLast: notRetryable, subscriptionLeft: retry, subscriptionLast: exhausted,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			left := httpcall.Attempt{Number: 1, Limit: 3, CreatedAt: queuedAt}
			last := httpcall.Attempt{Number: 3, Limit: 3, CreatedAt: queuedAt}
			now := queuedAt.Add(tc.age)

			oneOffLeft := httpcall.OneOff.Decide(tc.err, left, now)
			oneOffLast := httpcall.OneOff.Decide(tc.err, last, now)
			subscriptionLeft := httpcall.Subscription.Decide(tc.err, left, now)
			subscriptionLast := httpcall.Subscription.Decide(tc.err, last, now)

			assert.Equal(t, tc.oneOffLeft, oneOffLeft, "one-off call, attempts left")
			assert.Equal(t, tc.oneOffLast, oneOffLast, "one-off call, last attempt")
			assert.Equal(t, tc.subscriptionLeft, subscriptionLeft, "subscription, attempts left")
			assert.Equal(t, tc.subscriptionLast, subscriptionLast, "subscription, last attempt")
		})
	}
}

// failedAttempt draws any failure a call can end with, the attempt it ended and when.
type failedAttempt struct {
	err     error
	attempt httpcall.Attempt
	now     time.Time
}

func drawFailedAttempt(t *rapid.T) failedAttempt {
	kind := rapid.SampledFrom([]webhook.ErrorKind{webhook.KindTimeout, webhook.KindNetwork, webhook.KindBlockedAddress, webhook.KindHTTPStatus}).Draw(t, "kind")
	code := rapid.IntRange(300, 599).Draw(t, "status")
	retryAfter := rapid.SampledFrom([]string{"", "0", "120", "999999999", "soon", "Sun, 18 Oct 2026 09:02:00 GMT"}).Draw(t, "retryAfter")
	limit := rapid.IntRange(1, 30).Draw(t, "limit")
	number := rapid.IntRange(1, limit).Draw(t, "number")
	age := time.Duration(rapid.Int64Range(0, int64(2*webhook.HookRetryHorizon)).Draw(t, "age"))
	return failedAttempt{
		err:     &webhook.DeliveryError{Kind: kind, Status: code, RetryAfter: retryAfter},
		attempt: httpcall.Attempt{Number: number, Limit: limit, CreatedAt: queuedAt},
		now:     queuedAt.Add(age),
	}
}

func TestOnlyA429WaitsOrGivesUpAtTheHorizonUnderEitherPolicy(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		f := drawFailedAttempt(t)
		limited := webhook.RateLimited(f.err) != nil

		oneOff := httpcall.OneOff.Decide(f.err, f.attempt, f.now)
		subscription := httpcall.Subscription.Decide(f.err, f.attempt, f.now)

		oneOffHeldBack := oneOff.Outcome == httpcall.Wait || oneOff.Reason == httpcall.RateLimitedPastHorizon
		subscriptionHeldBack := subscription.Outcome == httpcall.Wait || subscription.Reason == httpcall.RateLimitedPastHorizon

		assert.Equal(t, limited, oneOffHeldBack)
		assert.Equal(t, limited, subscriptionHeldBack)
	})
}

func TestAWaitIsAlwaysPositiveAndAtMostTheLongestSnooze(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		f := drawFailedAttempt(t)

		d := httpcall.OneOff.Decide(f.err, f.attempt, f.now)
		inBounds := d.Outcome != httpcall.Wait || (d.Wait > 0 && d.Wait <= webhook.HookSnoozeMax)

		assert.True(t, inBounds, "wait %s", d.Wait)
	})
}

func TestTheTwoPoliciesDifferOnlyInWhatAOneOffCallCannotFix(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		f := drawFailedAttempt(t)

		oneOff := httpcall.OneOff.Decide(f.err, f.attempt, f.now)
		subscription := httpcall.Subscription.Decide(f.err, f.attempt, f.now)

		cannotBeFixed := webhook.RateLimited(f.err) == nil && !webhook.Retryable(f.err)
		agree := oneOff == subscription
		oneOffGaveUpAsNotRetryable := oneOff.Reason == httpcall.NotRetryable

		assert.NotEqual(t, httpcall.NotRetryable, subscription.Reason, "a subscription retries any failure")
		assert.True(t, agree || oneOffGaveUpAsNotRetryable, "one-off %+v, subscription %+v", oneOff, subscription)
		assert.Equal(t, cannotBeFixed, oneOffGaveUpAsNotRetryable)
	})
}

func TestAFailureOtherThanA429RetriesWhileAttemptsAreLeftAndGivesUpOnTheLast(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		f := drawFailedAttempt(t)
		code := rapid.IntRange(300, 599).Filter(func(c int) bool { return c != http.StatusTooManyRequests }).Draw(t, "status other than 429")
		err := status(code, "")
		last := f.attempt.Number == f.attempt.Limit

		d := httpcall.Subscription.Decide(err, f.attempt, f.now)
		gaveUp, retried := d == exhausted, d == retry

		assert.Equal(t, last, gaveUp)
		assert.Equal(t, !last, retried)
	})
}
