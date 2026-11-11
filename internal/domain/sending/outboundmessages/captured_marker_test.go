package outboundmessages_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// deliveringSender is a provider that says nothing about itself, as SMTP and SES do: it
// delivers outside.
type deliveringSender struct{}

func (deliveringSender) Send(context.Context, messaging.EmailMessage) (messaging.Receipt, error) {
	return messaging.Receipt{MessageID: "out-1"}, nil
}

func automationRequest(t *testing.T, env *testhelper.TestEnv, key string) outboundmessages.Request {
	t.Helper()
	req := marketing(t, env, key, fixtures.ContactAliceID)
	req.Source = eligibility.AutomationSource(fixtures.AutomationHoldDemoID)
	req.Ref = outboundmessages.AutomationRef{AutomationID: fixtures.AutomationHoldDemoID}
	return req
}

// The marker is the sender's own word that its mail stays inside: every surface carries it on the
// Outbound message and on the Events of the send, and a sender that delivers outside never does.
func TestASendThroughASenderThatKeepsItsMailIsMarkedCapturedOnEverySurface(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := moduleThrough(env, &keepingSender{confirms: true})
	requests := []outboundmessages.Request{
		transactional("cap:tx", "a@example.com"),
		marketing(t, env, "cap:bc", fixtures.ContactAliceID),
		automationRequest(t, env, "cap:auto"),
	}

	for _, req := range requests {
		surface := string(req.Kind())
		res, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), req)
		require.NoError(t, err, surface)
		require.Equal(t, outboundmessages.Sent, res.Outcome, surface)

		assert.True(t, env.DB.OutboundMessage.GetX(ctx, res.MessageID).Captured, surface)
		match := map[string]any{"outboundMessageId": res.MessageID, "captured": true}
		assert.Equal(t, 1, env.QueuedCount(t, events.NameEmailSent, match), surface)
		assert.Equal(t, 1, env.QueuedCount(t, events.NameEmailDelivered, match), surface)
	}
}

func TestASendThroughASenderThatDeliversOutsideIsNotMarkedCaptured(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := moduleThrough(env, deliveringSender{})

	req := transactional("cap:out", "a@example.com")
	req = sentFrom(req, "noreply@mail.acme.com") // fixture domain id 1: verified
	res, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), req)
	require.NoError(t, err)

	assert.False(t, env.DB.OutboundMessage.GetX(ctx, res.MessageID).Captured)
	assert.Zero(t, env.QueuedCount(t, events.NameEmailSent, map[string]any{"outboundMessageId": res.MessageID, "captured": true}))
	assert.Equal(t, 1, env.QueuedCount(t, events.NameEmailSent, map[string]any{"outboundMessageId": res.MessageID}))
}
