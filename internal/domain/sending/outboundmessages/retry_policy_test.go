package outboundmessages_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/kernel/deferralcause"
)

// A Deferral draws its own jitter, up to a fifth of the delay, so each case states the band
// the retry falls in: the delay itself, and that plus a fifth.

func deferral(wait time.Duration, backlog int) outboundmessages.Retry {
	return outboundmessages.Result{Outcome: outboundmessages.Deferral, Wait: wait}.Retry(backlog, time.Now())
}

// A Deferral waits the limit's wait (floored at a second) scaled by the recipients still
// ahead of it, capped at an hour, plus bounded jitter.
func TestADeferralRetriesAfterTheScaledWait(t *testing.T) {
	for _, c := range []struct {
		name    string
		wait    time.Duration
		backlog int
		base    time.Duration
	}{
		{"floored at a second", 100 * time.Millisecond, 0, time.Second},
		{"each one ahead adds a token's wait", 500 * time.Millisecond, 4, 2500 * time.Millisecond},
		{"14 per second, 99 ahead", 70 * time.Millisecond, 99, 7 * time.Second},
		{"capped so a raised limit is noticed", 12 * time.Hour, 3, time.Hour},
		// The product once overflowed int64, wrapped negative and woke the job after the floor.
		{"a long wait behind a huge backlog waits the ceiling", 48 * time.Hour, 52_916, time.Hour},
	} {
		retry := deferral(c.wait, c.backlog)

		assert.True(t, retry.Wanted, c.name)
		assert.GreaterOrEqual(t, retry.After, c.base, c.name)
		assert.LessOrEqual(t, retry.After, c.base+c.base/5, c.name)
	}
}

// For every wait and backlog the retry agrees with the model: the wait scaled by the
// recipients ahead, clamped to [1s, 1h], plus at most a fifth of that. So it never wakes
// sooner than the floor, never later than the ceiling plus jitter, and a longer backlog
// never makes it wake sooner.
func TestADeferralDelayAgreesWithTheModel(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		wait := time.Duration(rapid.Int64Range(0, int64(48*time.Hour)).Draw(t, "wait"))
		backlog := rapid.IntRange(0, 100_000).Draw(t, "backlog")

		retry := deferral(wait, backlog)

		base := min(max(float64(wait)*float64(1+backlog), float64(time.Second)), float64(time.Hour))
		assert.GreaterOrEqual(t, float64(retry.After), base-1)
		assert.LessOrEqual(t, float64(retry.After), base*1.2+1)
	})
}

// A Deferral caused by the Send cap asks again within the bound, jitter included, whatever the
// Workspace bucket's wait and the backlog ahead: a raised Send cap is noticed within minutes. It
// still never wakes sooner than the floor, and a longer backlog never makes it wake sooner.
func TestASendCapDeferralAlwaysRetriesWithinTheBound(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		wait := time.Duration(rapid.Int64Range(0, int64(48*time.Hour)).Draw(t, "wait"))
		backlog := rapid.IntRange(0, 100_000).Draw(t, "backlog")

		retry := outboundmessages.Result{Outcome: outboundmessages.Deferral, Cause: deferralcause.SendCap, Wait: wait}.Retry(backlog, time.Now())

		assert.True(t, retry.Wanted)
		assert.GreaterOrEqual(t, retry.After, time.Second)
		assert.LessOrEqual(t, retry.After, 10*time.Minute)
		base := min(max(float64(wait)*float64(1+backlog), float64(time.Second)), float64(10*time.Minute)/1.2)
		assert.GreaterOrEqual(t, float64(retry.After), base-1)
	})
}

// A per-recipient Deferral (a Frequency cap, a Sending window) wakes at its release moment plus at
// most a minute of jitter, however far off the moment is and whatever the backlog: neither the
// backlog nor a per-source ceiling moves it.
func TestAPerRecipientDeferralRetriesAtItsReleaseMoment(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		now := time.Date(2030, time.March, 4, 10, 0, 0, 0, time.UTC)
		ahead := time.Duration(rapid.Int64Range(1, int64(7*24*time.Hour)).Draw(t, "ahead"))
		backlog := rapid.IntRange(0, 100_000).Draw(t, "backlog")
		cause := rapid.SampledFrom([]deferralcause.Cause{deferralcause.FrequencyCap, deferralcause.SendingWindow}).Draw(t, "cause")
		until := now.Add(ahead)

		retry := outboundmessages.Result{Outcome: outboundmessages.Deferral, Cause: cause, Until: until}.Retry(backlog, now)

		assert.True(t, retry.Wanted)
		wake := now.Add(retry.After)
		assert.False(t, wake.Before(until), "never before the release moment")
		assert.Less(t, wake.Sub(until), time.Minute, "within a minute of it")
	})
}

// A 23-hour wait for a place under the Frequency cap is not cut to the hour a per-source Deferral is
// capped at.
func TestAPerRecipientDeferralIsNotCappedByThePerSourceCeiling(t *testing.T) {
	now := time.Date(2030, time.March, 4, 10, 0, 0, 0, time.UTC)

	retry := outboundmessages.Result{Outcome: outboundmessages.Deferral, Cause: deferralcause.FrequencyCap, Until: now.Add(23 * time.Hour)}.Retry(0, now)

	assert.GreaterOrEqual(t, retry.After, 23*time.Hour)
	assert.Less(t, retry.After, 23*time.Hour+time.Minute)
}
