package outbound_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/providerlimit"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Send cap (ADR 0067), observed at the Outbound send seam: the most messages a Workspace may
// send per rolling 24 hours, across its Integrations, on an Instance whose send path has Send caps.
// The clock stands still until the test moves it.

// sendCapBound is the longest a Deferral caused by the Send cap sleeps before asking again
// (spec #803: "on the order of 10 minutes"), so a raised Send cap is noticed within it.
const sendCapBound = 10 * time.Minute

// capped sets the Workspace's Send cap, as a Staff member would (nil clears it).
func capped(t *testing.T, env *testhelper.TestEnv, workspaceID int64, perDay *int) {
	t.Helper()
	upd := env.DB.Workspace.UpdateOneID(workspaceID)
	if perDay == nil {
		upd.ClearSendCap()
	} else {
		upd.SetSendCap(*perDay)
	}
	upd.ExecX(context.Background())
}

// sendCapModule is the send path of an Instance with Send caps, on the test's clock.
func sendCapModule(env *testhelper.TestEnv, c *clock) *outbound.Module {
	return newModule(env, outbound.WithSendCaps(true), outbound.WithClock(c.now))
}

func TestSpentSendCapIsADeferralWithTheWaitUntilTheWorkspaceBucketRefills(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := sendCapModule(env, c)
	capped(t, env, fixtures.AcmeID, new(2))

	require.Equal(t, outbound.Sent, send(t, m, env, "bc:1").Outcome)
	require.Equal(t, outbound.Sent, send(t, m, env, "bc:2").Outcome)

	res := send(t, m, env, "bc:3")
	assert.Equal(t, outbound.Deferral, res.Outcome)
	assert.Equal(t, providerlimit.CauseSendCap, res.Cause)
	assert.Empty(t, res.Reason, "a Deferral carries no hold reason")
	assert.Equal(t, 12*time.Hour, res.Wait, "2 per 24 hours refills one token every 12 hours")
	assert.Zero(t, res.MessageID)
	assert.Len(t, env.CustomerMail.Messages(), 2, "the provider is not called")

	c.elapse(res.Wait)
	assert.Equal(t, outbound.Sent, send(t, m, env, "bc:3").Outcome, "the window frees up gradually")
}

// However long the Workspace bucket takes to refill and however many sends wait behind this one,
// a Send cap Deferral asks again within the bound, jitter included, so a raised Send cap takes
// effect within minutes. A Provider limit Deferral keeps its own, longer ceiling.
func TestASendCapDeferralRetriesWithinTheBound(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := sendCapModule(env, c)
	capped(t, env, fixtures.AcmeID, new(1))

	require.Equal(t, outbound.Sent, send(t, m, env, "bc:1").Outcome)
	res := send(t, m, env, "bc:2")
	require.Equal(t, outbound.Deferral, res.Outcome)
	require.Equal(t, 24*time.Hour, res.Wait)

	for _, backlog := range []int{0, 1, 10_000} {
		retry := res.Retry(backlog)
		assert.True(t, retry.Wanted)
		assert.LessOrEqual(t, retry.After, sendCapBound, "backlog %d", backlog)
	}

	capped(t, env, fixtures.AcmeID, nil)
	limited(t, env, nil, new(1))
	require.Equal(t, outbound.Sent, send(t, m, env, "bc:2").Outcome)
	byProvider := send(t, m, env, "bc:3")
	require.Equal(t, outbound.Deferral, byProvider.Outcome)
	assert.Equal(t, providerlimit.CauseProviderLimit, byProvider.Cause)
	assert.Greater(t, byProvider.Retry(0).After, sendCapBound, "the Provider limit keeps its hour")
}

// A Staff member raising a spent Send cap speeds the Workspace up at once: the bucket refills at
// the new rate, so the next token comes in minutes instead of a day.
func TestARaisedSendCapRefillsAtTheNewRate(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := sendCapModule(env, c)
	capped(t, env, fixtures.AcmeID, new(1))

	require.Equal(t, outbound.Sent, send(t, m, env, "bc:1").Outcome)
	require.Equal(t, 24*time.Hour, send(t, m, env, "bc:2").Wait)

	capped(t, env, fixtures.AcmeID, new(1000))
	res := send(t, m, env, "bc:2")
	require.Equal(t, outbound.Deferral, res.Outcome)
	assert.Equal(t, 86400*time.Millisecond, res.Wait, "1000 per 24 hours refills one token every 86.4 seconds")

	c.elapse(res.Wait)
	assert.Equal(t, outbound.Sent, send(t, m, env, "bc:2").Outcome)
}

// Transactional mail is never delayed by the Send cap, yet it spends from it: the bucket goes into
// debt and later marketing sends wait longer.
func TestTransactionalSendsIgnoreTheSendCapButSpendIt(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := sendCapModule(env, c)
	capped(t, env, fixtures.AcmeID, new(2))
	ctx := context.Background()

	for i := range 3 {
		res, err := m.Send(ctx, acme(env), transactional(fmt.Sprintf("tx:%d", i), "buyer@example.com"))
		require.NoError(t, err)
		require.Equal(t, outbound.Sent, res.Outcome, "Transactional is never deferred")
	}

	res := send(t, m, env, "bc:1")
	require.Equal(t, outbound.Deferral, res.Outcome)
	assert.Equal(t, providerlimit.CauseSendCap, res.Cause)
	assert.Equal(t, 24*time.Hour, res.Wait, "three sends against a cap of two leave half a day of debt before the next token")
}

// The Workspace bucket and the Integration's buckets are spent together or not at all: a send the
// Provider limit defers leaves the Send cap's token, and one the Send cap defers leaves the
// Provider limit's.
func TestWorkspaceAndIntegrationBucketsAreSpentTogetherOrNotAtAll(t *testing.T) {
	t.Run("deferred by the Provider limit", func(t *testing.T) {
		env := testhelper.Setup(t)
		c := startClock()
		m := sendCapModule(env, c)
		capped(t, env, fixtures.AcmeID, new(2))
		limited(t, env, new(1), nil)

		require.Equal(t, outbound.Sent, send(t, m, env, "bc:1").Outcome)
		for range 3 {
			res := send(t, m, env, "bc:2")
			require.Equal(t, outbound.Deferral, res.Outcome)
			require.Equal(t, providerlimit.CauseProviderLimit, res.Cause)
		}

		c.elapse(time.Second)
		assert.Equal(t, outbound.Sent, send(t, m, env, "bc:2").Outcome, "the denied attempts left the Send cap's second token")
		c.elapse(time.Second)
		third := send(t, m, env, "bc:3")
		assert.Equal(t, outbound.Deferral, third.Outcome)
		assert.Equal(t, providerlimit.CauseSendCap, third.Cause, "now the Send cap is spent")
	})

	t.Run("deferred by the Send cap", func(t *testing.T) {
		env := testhelper.Setup(t)
		c := startClock()
		m := sendCapModule(env, c)
		capped(t, env, fixtures.AcmeID, new(1))
		limited(t, env, nil, new(2))

		require.Equal(t, outbound.Sent, send(t, m, env, "bc:1").Outcome)
		for range 3 {
			res := send(t, m, env, "bc:2")
			require.Equal(t, outbound.Deferral, res.Outcome)
			require.Equal(t, providerlimit.CauseSendCap, res.Cause)
		}

		capped(t, env, fixtures.AcmeID, nil)
		assert.Equal(t, outbound.Sent, send(t, m, env, "bc:2").Outcome, "the denied attempts left the Provider limit's second token")
		third := send(t, m, env, "bc:3")
		assert.Equal(t, outbound.Deferral, third.Outcome)
		assert.Equal(t, providerlimit.CauseProviderLimit, third.Cause)
	})
}

// A Workspace without a Send cap spends nothing from a Workspace bucket: when a cap is set later,
// the bucket is full whatever was sent before.
func TestSendsWithoutASendCapSpendNothingFromIt(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := sendCapModule(env, c)

	for i := range 3 {
		require.Equal(t, outbound.Sent, send(t, m, env, fmt.Sprintf("bc:free-%d", i)).Outcome)
	}

	capped(t, env, fixtures.AcmeID, new(3))
	for i := range 3 {
		require.Equal(t, outbound.Sent, send(t, m, env, fmt.Sprintf("bc:capped-%d", i)).Outcome)
	}
	assert.Equal(t, outbound.Deferral, send(t, m, env, "bc:over").Outcome)
}

// The Test inbox is an Integration like any other: its sends count against the Send cap.
func TestTestInboxSendsCountAgainstTheSendCap(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := globexInboxModule(t, env, c, outbound.WithSendCaps(true))
	capped(t, env, fixtures.GlobexID, new(2))

	require.Equal(t, outbound.Sent, sendToReader(t, env, m, "bc:1").Outcome)
	require.Equal(t, outbound.Sent, sendToReader(t, env, m, "bc:2").Outcome)

	res := sendToReader(t, env, m, "bc:3")
	assert.Equal(t, outbound.Deferral, res.Outcome, "the Test inbox's own 200 a day is far from spent")
	assert.Equal(t, providerlimit.CauseSendCap, res.Cause)
}

// The Instance's own send path enforces the Send cap on an Instance that has Send caps.
func TestTheInstanceSendPathEnforcesTheSendCap(t *testing.T) {
	env := testhelper.Setup(t)
	m := env.OutboundModule()
	capped(t, env, fixtures.AcmeID, new(1))

	require.Equal(t, outbound.Sent, send(t, m, env, "bc:1").Outcome)
	res := send(t, m, env, "bc:2")
	assert.Equal(t, outbound.Deferral, res.Outcome)
	assert.Equal(t, providerlimit.CauseSendCap, res.Cause)
}

// A self-hosted Instance has no Send cap: a value stored on the Workspace (a restored dump, say)
// has no effect on sending.
func TestASelfHostedInstanceIgnoresAStoredSendCap(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithMode(config.ModeSelfHosted))
	m := env.OutboundModule()
	capped(t, env, fixtures.AcmeID, new(1))

	for i := range 3 {
		assert.Equal(t, outbound.Sent, send(t, m, env, fmt.Sprintf("bc:%d", i)).Outcome)
	}
}
