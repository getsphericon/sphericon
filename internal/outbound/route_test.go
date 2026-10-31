package outbound_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The route of the Workspace's default carries its Integration, its sender and its pace.
func TestTheRouteOfTheDefaultCarriesItsIntegrationAndPace(t *testing.T) {
	env := testhelper.Setup(t)
	limited(t, env, new(4), nil)

	route, err := newModule(env).Route(context.Background(), acme(env), nil)

	require.NoError(t, err)
	assert.False(t, route.Held())
	require.NotNil(t, route.Integration)
	assert.Equal(t, int64(fixtures.IntegrationAcmeDefaultID), route.Integration.ID)
	assert.NotNil(t, route.Sender)
	assert.True(t, route.DeliversOutside)
	assert.Equal(t, 250*time.Millisecond, route.Interval())
	assert.NoError(t, route.Armable())
}

// A named Integration that cannot send holds its route, and never falls back to the default:
// the route cannot be armed and has no pace.
func TestTheRouteOfANamedIntegrationThatCannotSendIsHeld(t *testing.T) {
	cases := map[string]int64{
		"disabled": fixtures.IntegrationAcmeSesID,
		"archived": fixtures.IntegrationAcmeArchivedID,
		"foreign":  fixtures.IntegrationGlobexID,
	}
	for name, id := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)

			route, err := newModule(env).Route(context.Background(), acme(env), &id)

			require.NoError(t, err)
			assert.Equal(t, outbound.HoldNoIntegration, route.Hold)
			assert.Nil(t, route.Integration, "not the default in its place")
			assert.Zero(t, route.Interval())
			assert.ErrorIs(t, route.Armable(), outbound.ErrRouteHeld)
		})
	}
}

// A send through a disabled named Integration is held for the route's reason, even though the
// Workspace's default could send: nothing is claimed and nothing goes out.
func TestASendThroughADisabledNamedIntegrationIsHeld(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env)
	req := marketing(t, env, "bc:disabled", fixtures.ContactAliceID)
	req.IntegrationID = new(int64(fixtures.IntegrationAcmeSesID))

	res, err := m.Send(context.Background(), acme(env), req)

	require.NoError(t, err)
	assert.Equal(t, outbound.Held, res.Outcome)
	route, err := m.Route(context.Background(), acme(env), req.IntegrationID)
	require.NoError(t, err)
	assert.Equal(t, route.Hold, res.Reason, "the send holds for the reason its route gives")
	assert.Zero(t, res.MessageID)
	assert.Empty(t, env.CustomerMail.Messages())
	hold, err := m.Preflight(context.Background(), acme(env), "", req.IntegrationID)
	require.NoError(t, err)
	assert.Equal(t, route.Hold, hold, "the plan's Preflight agrees")
}

// The Provider limit runs on the module's clock: a spent limit defers until the clock, not the
// wall, has moved on by the wait.
func TestTheProviderLimitRunsOnTheInjectedClock(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, outbound.WithClock(c.now))
	limited(t, env, new(1), nil)
	require.Equal(t, outbound.Sent, send(t, m, env, "bc:1").Outcome)

	res := send(t, m, env, "bc:2")
	require.Equal(t, outbound.Deferral, res.Outcome)
	c.elapse(res.Wait - time.Millisecond)
	early := send(t, m, env, "bc:2")
	c.elapse(time.Millisecond)
	due := send(t, m, env, "bc:2")

	assert.Equal(t, outbound.Deferral, early.Outcome, "a millisecond short of the wait")
	assert.Equal(t, outbound.Sent, due.Outcome)
	msg, err := acme(env).OutboundMessage().Get(context.Background(), due.MessageID)
	require.NoError(t, err)
	require.NotNil(t, msg.SentAt)
	assert.WithinDuration(t, c.now(), *msg.SentAt, time.Microsecond, "recorded sent at the module's clock")
	assert.Equal(t, outboundmessage.StatusSent, msg.Status)
}

// A provider that has stopped the account holds the route but does not refuse arming a send: the
// standing lifts by itself, and the sends wait for it. A route with no Integration that can send is
// refused.
func TestAStoppedProviderHoldsTheRouteButArmingStillPasses(t *testing.T) {
	assert.NoError(t, outbound.Route{Hold: outbound.HoldProviderStopped}.Armable())
	assert.ErrorIs(t, outbound.Route{Hold: outbound.HoldNoIntegration}.Armable(), outbound.ErrRouteHeld)
}
