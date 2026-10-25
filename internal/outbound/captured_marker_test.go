package outbound_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// deliveringSender is a provider that says nothing about itself, as SMTP and SES do: it
// delivers outside.
type deliveringSender struct{}

func (deliveringSender) Send(context.Context, messaging.EmailMessage) (messaging.Receipt, error) {
	return messaging.Receipt{MessageID: "out-1"}, nil
}

func automationRequest(t *testing.T, env *testhelper.TestEnv, key string) outbound.Request {
	t.Helper()
	req := marketing(t, env, key, fixtures.ContactAliceID)
	req.Kind = outboundmessage.KindAutomation
	req.Source = eligibility.AutomationSource(fixtures.AutomationHoldDemoID)
	req.Ref.AutomationID = fixtures.AutomationHoldDemoID
	return req
}

// The marker is the sender's own word that its mail stays inside: every surface carries it on the
// Outbound message and on the Events of the send, and a sender that delivers outside never does.
func TestASendThroughASenderThatKeepsItsMailIsMarkedCapturedOnEverySurface(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := moduleThrough(env, &keepingSender{confirms: true})
	requests := []outbound.Request{
		transactional("cap:tx", "a@example.com"),
		marketing(t, env, "cap:bc", fixtures.ContactAliceID),
		automationRequest(t, env, "cap:auto"),
	}

	for _, req := range requests {
		surface := string(req.Kind)
		res, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), req)
		require.NoError(t, err, surface)
		require.Equal(t, outbound.Sent, res.Outcome, surface)

		assert.True(t, env.DB.OutboundMessage.GetX(ctx, res.MessageID).Captured, surface)
		match := map[string]any{"outboundMessageId": res.MessageID, "captured": true}
		assert.Equal(t, 1, env.OutboxCount(t, events.NameEmailSent, match), surface)
		assert.Equal(t, 1, env.OutboxCount(t, events.NameEmailDelivered, match), surface)
	}
}

func TestASendThroughASenderThatDeliversOutsideIsNotMarkedCaptured(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := moduleThrough(env, deliveringSender{})

	req := transactional("cap:out", "a@example.com")
	req.FromEmail = "noreply@mail.acme.com" // fixture domain id 1: verified
	res, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), req)
	require.NoError(t, err)

	assert.False(t, env.DB.OutboundMessage.GetX(ctx, res.MessageID).Captured)
	assert.Zero(t, env.OutboxCount(t, events.NameEmailSent, map[string]any{"outboundMessageId": res.MessageID, "captured": true}))
	assert.Equal(t, 1, env.OutboxCount(t, events.NameEmailSent, map[string]any{"outboundMessageId": res.MessageID}))
}
