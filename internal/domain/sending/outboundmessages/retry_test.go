package outboundmessages_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
)

// Only a Hold and a Deferral want a retry: every other Outcome is final.
func TestOnlyAHoldOrADeferralWantsARetry(t *testing.T) {
	for _, o := range []outboundmessages.Outcome{outboundmessages.Sent, outboundmessages.Skipped, outboundmessages.Failed, outboundmessages.Refused} {
		retry := outboundmessages.Result{Outcome: o}.Retry(0)
		assert.False(t, retry.Wanted, o)
	}
	hold := outboundmessages.Result{Outcome: outboundmessages.Held}.Retry(0)
	deferral := outboundmessages.Result{Outcome: outboundmessages.Deferral, Wait: time.Second}.Retry(0)
	assert.True(t, hold.Wanted)
	assert.True(t, deferral.Wanted)
}

// A Hold retries after a fixed backoff whatever the source, so a Broadcast recipient and an
// Automation run held by the same freeze come back on the same schedule.
func TestAHoldRetriesAfterTheHoldBackoffForEverySource(t *testing.T) {
	for _, reason := range []string{holds.ReasonSuspended, holds.ReasonStaff, outboundmessages.HoldNoIntegration} {
		retry := outboundmessages.Result{Outcome: outboundmessages.Held, Reason: reason}.Retry(0)
		assert.Equal(t, 15*time.Minute, retry.After, reason)
	}
	hold := outboundmessages.HoldRetry()
	assert.Equal(t, outboundmessages.HoldBackoff, hold.After)
}

// A Deferral's jitter adds at most a fifth of the delay, so the drawn retry stays inside it.
func TestADeferralResultRetriesWithinTheJitterBand(t *testing.T) {
	retry := outboundmessages.Result{Outcome: outboundmessages.Deferral, Wait: time.Second}.Retry(4)
	assert.GreaterOrEqual(t, retry.After, 5*time.Second)
	assert.LessOrEqual(t, retry.After, 6*time.Second)
}
