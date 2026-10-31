package outbound_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/capturedmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/inbox"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// The Send cap (ADR 0067), observed at the Outbound send seam on a clock that stands still until
// the test moves it. A Staff member sets it through the suspension module, as the console does.

// capped sets the Send cap of the Workspace.
func capped(t *testing.T, env *testhelper.TestEnv, workspaceID int64, sendCap int) {
	t.Helper()
	_, err := suspension.SetSendCap(context.Background(), env.Bus, workspaceID, suspension.StaffMember("op-1"), &sendCap)
	require.NoError(t, err)
}

func TestASpentSendCapDefersMarketingWithABoundedRetry(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, outbound.WithClock(c.now), outbound.WithSendCap())
	capped(t, env, fixtures.AcmeID, 2)

	require.Equal(t, outbound.Sent, send(t, m, env, "bc:1").Outcome)
	require.Equal(t, outbound.Sent, send(t, m, env, "bc:2").Outcome)
	res := send(t, m, env, "bc:3")

	require.Equal(t, outbound.Deferral, res.Outcome, "a spent Send cap slows, it does not stop")
	assert.True(t, res.SendCap, "the Send cap, not the Provider limit, is what is spent")
	assert.Equal(t, 12*time.Hour, res.Wait, "2 per 24 hours returns a token every 12 hours")
	assert.LessOrEqual(t, res.Retry(0).After, 12*time.Minute, "asked again within minutes, so a raised Send cap shows soon")
	assert.Len(t, env.CustomerMail.Messages(), 2, "the provider is not called")

	c.elapse(res.Wait)
	assert.Equal(t, outbound.Sent, send(t, m, env, "bc:3").Outcome, "sent once the window frees up")
}

func TestTransactionalIsNeverDelayedBySendCapButSpendsIt(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, outbound.WithClock(c.now), outbound.WithSendCap())
	capped(t, env, fixtures.AcmeID, 1)

	for _, key := range []string{"tx:1", "tx:2"} {
		res, err := m.Send(context.Background(), acme(env), transactional(key, "receipt@example.com"))
		require.NoError(t, err)
		require.Equal(t, outbound.Sent, res.Outcome, key)
	}
	res := send(t, m, env, "bc:1")

	require.Equal(t, outbound.Deferral, res.Outcome)
	assert.Equal(t, 48*time.Hour, res.Wait, "two Transactional sends left the 1-a-day bucket a token in debt")
}

func TestSendCapAndProviderLimitAreSpentTogetherOrNotAtAll(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, outbound.WithClock(c.now), outbound.WithSendCap())
	capped(t, env, fixtures.AcmeID, 2)
	limited(t, env, new(1), nil)

	require.Equal(t, outbound.Sent, send(t, m, env, "bc:1").Outcome)
	busy := send(t, m, env, "bc:2")
	require.Equal(t, outbound.Deferral, busy.Outcome)
	assert.False(t, busy.SendCap, "the Provider limit is what is spent")

	c.elapse(time.Second)
	assert.Equal(t, outbound.Sent, send(t, m, env, "bc:2").Outcome, "the refused attempt took no token of the Send cap")
	c.elapse(time.Second)
	assert.True(t, send(t, m, env, "bc:3").SendCap, "two sent, the Send cap is spent")
}

func TestAStoredSendCapHasNoEffectWithoutSendCaps(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, outbound.WithClock(c.now))
	capped(t, env, fixtures.AcmeID, 1)

	for _, key := range []string{"bc:1", "bc:2", "bc:3"} {
		assert.Equal(t, outbound.Sent, send(t, m, env, key).Outcome, key)
	}
	assert.Zero(t, acme(env).SendLimiter().Query().CountX(context.Background()), "no bucket is written")
}

func TestNoSendCapCostsNoWrite(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env, outbound.WithClock(startClock().now), outbound.WithSendCap())

	require.Equal(t, outbound.Sent, send(t, m, env, "bc:1").Outcome)

	assert.Zero(t, acme(env).SendLimiter().Query().CountX(context.Background()))
}

func TestTestInboxSendsCountAgainstTheSendCap(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	s := env.DB.Scoped(fixtures.GlobexID)
	built, err := inbox.Descriptor().Build(nil, messaging.Deps{
		Capturer: capturedmessages.New(inbox.PerDay).Capturer(s, fixtures.IntegrationGlobexInboxID),
	})
	require.NoError(t, err)
	sender, ok := built.(messaging.EmailSender)
	require.True(t, ok)
	env.DB.Integration.UpdateOneID(fixtures.IntegrationGlobexInboxID).SetIsDefault(true).ExecX(context.Background())
	env.DB.Integration.UpdateOneID(fixtures.IntegrationGlobexID).SetIsDefault(false).ExecX(context.Background())
	m := outbound.New(env.Bus, env.Holds, senders{sender: sender}, tracking.New("test-secret", "http://local"), outbound.WithClock(c.now), outbound.WithSendCap())
	capped(t, env, fixtures.GlobexID, 1)

	require.Equal(t, outbound.Sent, sendToReader(t, env, m, "bc:1").Outcome)
	res := sendToReader(t, env, m, "bc:2")

	require.Equal(t, outbound.Deferral, res.Outcome)
	assert.True(t, res.SendCap)
}
