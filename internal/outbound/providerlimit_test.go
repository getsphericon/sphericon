package outbound_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/providerlimiter"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/providerlimit"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Provider limit (ADR 0023), observed at the Outbound send seam on an injected clock that stands
// still until the test moves it. The default Acme Integration (fixture) carries the limits.

// limited sets the Provider limit of the Acme default Integration (nil = none).
func limited(t *testing.T, env *testhelper.TestEnv, perSecond, perDay *int) {
	t.Helper()
	upd := env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID)
	if perSecond != nil {
		upd.SetMaxPerSecond(*perSecond)
	}
	if perDay != nil {
		upd.SetMaxPerDay(*perDay)
	}
	upd.ExecX(context.Background())
}

func send(t *testing.T, m *outbound.Module, env *testhelper.TestEnv, key string) outbound.Result {
	t.Helper()
	res, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), marketing(t, env, key, fixtures.ContactAliceID))
	require.NoError(t, err)
	return res
}

func acme(env *testhelper.TestEnv) *ent.Scoped { return env.DB.Scoped(fixtures.AcmeID) }

func TestSpentPerSecondLimitIsADeferralWithTheWaitUntilCapacityReturns(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, outbound.WithClock(c.now))
	limited(t, env, new(2), nil)

	assert.Equal(t, outbound.Sent, send(t, m, env, "bc:1").Outcome)
	assert.Equal(t, outbound.Sent, send(t, m, env, "bc:2").Outcome)

	res := send(t, m, env, "bc:3")
	assert.Equal(t, outbound.Deferral, res.Outcome)
	assert.Empty(t, res.Reason, "a Deferral carries no hold reason")
	assert.Equal(t, 500*time.Millisecond, res.Wait, "one token at 2 per second takes half a second")
	assert.Zero(t, res.MessageID)
	assert.Len(t, env.CustomerMail.Messages(), 2, "the provider is not called")
}

func TestDeferredSendLeavesNoClaimAndSendsExactlyOnceAfterTheWait(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, outbound.WithClock(c.now))
	limited(t, env, new(1), nil)

	require.Equal(t, outbound.Sent, send(t, m, env, "bc:a").Outcome)
	res := send(t, m, env, "bc:b")
	require.Equal(t, outbound.Deferral, res.Outcome)
	assert.Equal(t, time.Second, res.Wait)

	n, err := acme(env).OutboundMessage().Query().Count(context.Background())
	require.NoError(t, err)
	before := n
	// The deferred key left no row: no claim, so nothing for a retry to trip on.
	c.elapse(res.Wait)
	retry := send(t, m, env, "bc:b")
	assert.Equal(t, outbound.Sent, retry.Outcome)
	assert.False(t, retry.Replayed)

	n, err = acme(env).OutboundMessage().Query().Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, before+1, n, "exactly one message row for the deferred key")
	assert.Len(t, env.CustomerMail.Messages(), 2)
}

func TestDailyLimitIsRollingAndReportsItsOwnWait(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, outbound.WithClock(c.now))
	limited(t, env, nil, new(2))

	require.Equal(t, outbound.Sent, send(t, m, env, "bc:1").Outcome)
	require.Equal(t, outbound.Sent, send(t, m, env, "bc:2").Outcome)

	res := send(t, m, env, "bc:3")
	require.Equal(t, outbound.Deferral, res.Outcome)
	assert.Equal(t, 12*time.Hour, res.Wait, "2 per 24h refills one token every 12 hours")

	c.elapse(12 * time.Hour)
	assert.Equal(t, outbound.Sent, send(t, m, env, "bc:3").Outcome, "capacity returns gradually, not at a window edge")
	assert.Equal(t, outbound.Deferral, send(t, m, env, "bc:4").Outcome)
}

func TestReservationTakesFromBothBucketsOrNone(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, outbound.WithClock(c.now))
	limited(t, env, new(1), new(2))

	require.Equal(t, outbound.Sent, send(t, m, env, "bc:1").Outcome) // second 0, day 1
	require.Equal(t, outbound.Deferral, send(t, m, env, "bc:2").Outcome)
	require.Equal(t, outbound.Deferral, send(t, m, env, "bc:2").Outcome)

	// Had the denials taken a daily token, the daily bucket would now be empty.
	c.elapse(time.Second)
	assert.Equal(t, outbound.Sent, send(t, m, env, "bc:2").Outcome, "the denied attempts spent no daily capacity")
	c.elapse(time.Second)
	assert.Equal(t, outbound.Deferral, send(t, m, env, "bc:3").Outcome, "the daily bucket is now spent")
}

func TestSkippedAndHeldMessagesSpendNoCapacity(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, outbound.WithClock(c.now))
	limited(t, env, new(1), nil)
	ctx := context.Background()

	// Skipped: the destination is suppressed.
	sup := marketing(t, env, "bc:sup", fixtures.ContactAliceID)
	sup.Destination = suppressd
	res, err := m.Send(ctx, acme(env), sup)
	require.NoError(t, err)
	require.Equal(t, outbound.Skipped, res.Outcome)

	// Held: an unverified Sending domain.
	held := marketing(t, env, "bc:held", fixtures.ContactAliceID)
	held = sentFrom(held, "noreply@news.acme.com")
	res, err = m.Send(ctx, acme(env), held)
	require.NoError(t, err)
	require.Equal(t, outbound.Held, res.Outcome)

	assert.Equal(t, outbound.Sent, send(t, m, env, "bc:ok").Outcome, "the one token was still there")
}

func TestNoLimitMeansNoLimiterStateAndNoDeferral(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, outbound.WithClock(c.now))

	for _, key := range []string{"bc:1", "bc:2", "bc:3", "bc:4", "bc:5"} {
		assert.Equal(t, outbound.Sent, send(t, m, env, key).Outcome)
	}
	n, err := env.DB.ProviderLimiter.Query().Where(providerlimiter.IntegrationID(fixtures.IntegrationAcmeDefaultID)).Count(context.Background())
	require.NoError(t, err)
	assert.Zero(t, n)
}

func TestTransactionalSendsAreNeverDeferredEvenWithTheLimitSpent(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, outbound.WithClock(c.now))
	limited(t, env, new(1), nil)

	require.Equal(t, outbound.Sent, send(t, m, env, "bc:1").Outcome)
	require.Equal(t, outbound.Deferral, send(t, m, env, "bc:2").Outcome)
	for _, key := range []string{"tx:1", "tx:2", "tx:3"} {
		res, err := m.Send(context.Background(), acme(env), transactional(key, "a@example.com"))
		require.NoError(t, err)
		assert.Equal(t, outbound.Sent, res.Outcome)
		assert.Zero(t, res.Wait)
	}
}

func TestTransactionalSendsSpendCapacityAndDelayLaterMarketingSends(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, outbound.WithClock(c.now))
	limited(t, env, new(1), nil)
	ctx := context.Background()

	// Three Transactional sends drive the per-second bucket to minus two tokens.
	for _, key := range []string{"tx:1", "tx:2", "tx:3"} {
		res, err := m.Send(ctx, acme(env), transactional(key, "a@example.com"))
		require.NoError(t, err)
		require.Equal(t, outbound.Sent, res.Outcome)
	}

	res := send(t, m, env, "bc:1")
	require.Equal(t, outbound.Deferral, res.Outcome)
	assert.Equal(t, 3*time.Second, res.Wait, "debt of two tokens plus the one the Broadcast needs")

	c.elapse(2 * time.Second)
	assert.Equal(t, outbound.Deferral, send(t, m, env, "bc:1").Outcome)
	c.elapse(time.Second)
	assert.Equal(t, outbound.Sent, send(t, m, env, "bc:1").Outcome)
}

func TestTransactionalSendsCountTowardTheDailyBucketToo(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, outbound.WithClock(c.now))
	limited(t, env, nil, new(2))

	for _, key := range []string{"tx:1", "tx:2", "tx:3"} {
		res, err := m.Send(context.Background(), acme(env), transactional(key, "a@example.com"))
		require.NoError(t, err)
		require.Equal(t, outbound.Sent, res.Outcome)
	}
	res := send(t, m, env, "bc:1")
	require.Equal(t, outbound.Deferral, res.Outcome)
	assert.Equal(t, 24*time.Hour, res.Wait, "2 per 24h: two tokens of debt plus one more, 12 hours each")
}

func TestTransactionalThatIsSkippedSpendsNoCapacity(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, outbound.WithClock(c.now))
	limited(t, env, new(1), nil)

	res, err := m.Send(context.Background(), acme(env), transactional("tx:sup", suppressd))
	require.NoError(t, err)
	require.Equal(t, outbound.Skipped, res.Outcome)

	assert.Equal(t, outbound.Sent, send(t, m, env, "bc:1").Outcome, "the suppressed Transactional took no token")
}

func TestUnlimitedIntegrationKeepsNoStateForTransactional(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, outbound.WithClock(c.now))

	res, err := m.Send(context.Background(), acme(env), transactional("tx:1", "a@example.com"))
	require.NoError(t, err)
	require.Equal(t, outbound.Sent, res.Outcome)
	n, err := env.DB.ProviderLimiter.Query().Where(providerlimiter.IntegrationID(fixtures.IntegrationAcmeDefaultID)).Count(context.Background())
	require.NoError(t, err)
	assert.Zero(t, n)
}

func TestAutomationAndBroadcastSendsShareOneLimit(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, outbound.WithClock(c.now))
	limited(t, env, new(1), nil)

	auto := marketing(t, env, "auto:1", fixtures.ContactAliceID)
	auto.Kind = outboundmessage.KindAutomation
	auto.Source = eligibility.AutomationSource(fixtures.AutomationHoldDemoID)
	res, err := m.Send(context.Background(), acme(env), auto)
	require.NoError(t, err)
	require.Equal(t, outbound.Sent, res.Outcome)

	assert.Equal(t, outbound.Deferral, send(t, m, env, "bc:1").Outcome, "the Automation send spent the Broadcast's token")
}

func TestChangedLimitTakesEffectImmediatelyAndProportionally(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, outbound.WithClock(c.now))
	limited(t, env, new(10), nil)

	require.Equal(t, outbound.Sent, send(t, m, env, "bc:1").Outcome) // bucket 9/10 full
	limited(t, env, new(1), nil)

	res := send(t, m, env, "bc:2") // 0.9 of one token: not enough yet
	assert.Equal(t, outbound.Deferral, res.Outcome)
	assert.Equal(t, 100*time.Millisecond, res.Wait)
}

func TestSentMessageIsStampedWithItsIntegrationAndCountsAsUsage(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, outbound.WithClock(c.now))

	before, err := providerlimit.Usage(context.Background(), acme(env), time.Now())
	require.NoError(t, err)

	res := send(t, m, env, "bc:usage")
	require.Equal(t, outbound.Sent, res.Outcome)

	msg, err := acme(env).OutboundMessage().Get(context.Background(), res.MessageID)
	require.NoError(t, err)
	require.NotNil(t, msg.IntegrationID, "the message records the Integration it went through")
	assert.Equal(t, int64(fixtures.IntegrationAcmeDefaultID), *msg.IntegrationID)

	after, err := providerlimit.Usage(context.Background(), acme(env), time.Now())
	require.NoError(t, err)
	assert.Equal(t, before[fixtures.IntegrationAcmeDefaultID]+1, after[fixtures.IntegrationAcmeDefaultID])
}
