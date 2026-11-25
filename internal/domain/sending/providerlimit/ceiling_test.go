package providerlimit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/deferralcause"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A bucket holds messages and its balance is never rescaled (ADR 0023, #1808): the time since the
// last write refills at the ceiling in force at the call, capped at it, so a debt stays the same
// number of messages. The clock stands still unless a test moves it.

// perDay is a reservation through Acme's default Integration under a Provider limit of perDay
// messages per 24 hours.
func perDay(n int) providerlimit.Reservation {
	return providerlimit.Reservation{IntegrationID: fixtures.IntegrationAcmeDefaultID, Limits: providerlimit.Limits{PerDay: new(n)}}
}

// granted reserves r until it is denied, at most limit times, and reports how many were granted
// and the denial's wait.
func granted(t *testing.T, b *providerlimit.Buckets, env *testhelper.TestEnv, r providerlimit.Reservation, limit int) (int, time.Duration) {
	t.Helper()
	s := env.DB.Scoped(fixtures.AcmeID)
	for n := range limit {
		short, err := b.Reserve(t.Context(), s, r)
		require.NoError(t, err)
		if short.Wait != 0 {
			assert.Equal(t, deferralcause.ProviderLimit, short.Cause)
			return n, short.Wait
		}
	}
	return limit, 0
}

func TestARaisedProviderLimitKeepsATransactionalDebtInMessages(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	b := providerlimit.NewBuckets(env.Bus.WithinAuditTx, func() time.Time { return now })
	for range 3 {
		require.NoError(t, b.Spend(t.Context(), env.DB.Scoped(fixtures.AcmeID), perDay(2)))
	}
	_, before := granted(t, b, env, perDay(2), 1)
	require.Equal(t, 24*time.Hour, before, "one message of debt and one to send, at 12 hours each")

	_, after := granted(t, b, env, perDay(1000), 1)

	assert.Equal(t, 172800*time.Millisecond, after, "the same two tokens, at 86.4 seconds each")
}

func TestARaisedCeilingKeepsTheBalance(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	b := providerlimit.NewBuckets(env.Bus.WithinAuditTx, func() time.Time { return now })
	n, _ := granted(t, b, env, perDay(100), 1)
	require.Equal(t, 1, n, "the first message of a full bucket of 100")

	n, wait := granted(t, b, env, perDay(1000), 1000)

	assert.Equal(t, 99, n, "the rest of the 100 at once")
	assert.Equal(t, 86400*time.Millisecond, wait, "then one every 86.4 seconds")
	now = now.Add(wait)
	n, _ = granted(t, b, env, perDay(1000), 2)
	assert.Equal(t, 1, n)
}

func TestALoweredCeilingCapsTheTokens(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	b := providerlimit.NewBuckets(env.Bus.WithinAuditTx, func() time.Time { return now })
	n, _ := granted(t, b, env, perDay(1000), 1)
	require.Equal(t, 1, n, "the first message of a full bucket of 1000, 999 left")

	n, wait := granted(t, b, env, perDay(100), 1000)

	assert.Equal(t, 100, n, "at most the new ceiling at once")
	assert.Equal(t, 864*time.Second, wait, "then one every 864 seconds")
}

// sesPerDay is a reservation through Acme's SES Integration, whose limiter is a fixture, under a
// Provider limit of n messages per 24 hours.
func sesPerDay(n int) providerlimit.Reservation {
	return providerlimit.Reservation{IntegrationID: fixtures.IntegrationAcmeSesID, Limits: providerlimit.Limits{PerDay: new(n)}}
}

// emptied leaves the fixture limiter of Acme's SES Integration one token at now and spends it under
// a Provider limit of ceiling per day, so the bucket was last written empty at that ceiling.
func emptied(t *testing.T, b *providerlimit.Buckets, env *testhelper.TestEnv, now time.Time, ceiling int) {
	t.Helper()
	require.NoError(t, env.DB.ProviderLimiter.UpdateOneID(fixtures.ProviderLimiterAcmeID).
		SetDayTokens(1).SetRefilledAt(now).Exec(t.Context()))
	n, _ := granted(t, b, env, sesPerDay(ceiling), 1)
	require.Equal(t, 1, n, "the one token at the starting ceiling")
}

// The time since the last write refills at the ceiling in force at the call: a bucket emptied at
// 100, idle for 12 hours, then raised to 10 000 holds half a day at the new rate.
func TestARaiseAfterIdleTimeRefillsItAtTheNewCeiling(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	b := providerlimit.NewBuckets(env.Bus.WithinAuditTx, func() time.Time { return now })
	emptied(t, b, env, now, 100)
	now = now.Add(12 * time.Hour)

	n, _ := granted(t, b, env, sesPerDay(10_000), 10_000)

	assert.Equal(t, 5_000, n)
}

// The same for a lowered ceiling: a bucket emptied at 10 000, idle for 12 hours, then lowered to
// 100 holds half a day at 100.
func TestALoweringAfterIdleTimeRefillsItAtTheNewCeiling(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	b := providerlimit.NewBuckets(env.Bus.WithinAuditTx, func() time.Time { return now })
	emptied(t, b, env, now, 10_000)
	now = now.Add(12 * time.Hour)

	n, _ := granted(t, b, env, sesPerDay(100), 100)

	assert.Equal(t, 50, n)
}
