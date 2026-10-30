package events_test

import (
	"encoding/json"
	"testing"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/events"
)

func rawMsg(payload string) *message.Message {
	return message.NewMessage(watermill.NewUUID(), []byte(payload))
}

func envelopeOf(name, data string) events.Envelope {
	return events.Envelope{ID: "evt", Name: name, Version: 1, WorkspaceID: 1, Data: json.RawMessage(data)}
}

func envelopeMsg(t *testing.T, name, data string) *message.Message {
	t.Helper()
	body, err := json.Marshal(envelopeOf(name, data))
	require.NoError(t, err)
	return rawMsg(string(body))
}

// Every consumer rejects a payload that is not an envelope and an envelope it
// cannot decode, so the router retries or parks the message instead of dropping it
// silently. (The error comes before the consumer touches its dependency, hence nil.)
func TestConsumersRejectUndecodableMessages(t *testing.T) {
	consumers := map[string]message.NoPublishHandlerFunc{
		"webhooks":    events.WebhooksConsumer(nil, nil),
		"automations": events.AutomationsConsumer(nil),
		"rules":       events.RulesConsumer(nil, nil),
	}
	for name, handle := range consumers {
		require.Error(t, handle(rawMsg("not json")), name+": not an envelope")
		require.ErrorContains(t, handle(envelopeMsg(t, "no.such.event", "{}")), "unknown event", name)
		require.ErrorContains(t, handle(envelopeMsg(t, events.NameContactCreated, `"scalar"`)), "decode", name)
	}
}

// The persist and suppression consumers hand the envelope to Persist and Suppress, which
// refuse one they cannot decode the same way.
func TestPersistAndSuppressRejectUndecodableEnvelopes(t *testing.T) {
	ctx := t.Context()
	require.ErrorContains(t, events.Persist(ctx, nil, envelopeOf("no.such.event", "{}")), "unknown event")
	require.ErrorContains(t, events.Persist(ctx, nil, envelopeOf(events.NameContactCreated, `"scalar"`)), "decode")
	require.ErrorContains(t, events.Suppress(ctx, nil, envelopeOf("no.such.event", "{}")), "unknown event")
	require.ErrorContains(t, events.Suppress(ctx, nil, envelopeOf(events.NameContactCreated, `"scalar"`)), "decode")
}

func TestDecodeKnowsEveryEventName(t *testing.T) {
	for _, name := range []string{
		events.NameContactCreated,
		events.NameContactErased,
		events.NameEmailSent,
		events.NameEmailDelivered,
		events.NameEmailDelayed,
		events.NameEmailDropped,
		events.NameEmailOpened,
		events.NameEmailClicked,
		events.NameEmailUnsubscribed,
		events.NameEmailBounced,
		events.NameEmailComplained,
		events.NameMarketingConfirmed,
		events.NameCollected,
		events.NameAuditEntry,
		events.NameTrackingDomainChanged,
	} {
		ev, err := events.Decode(events.Envelope{Name: name, Data: []byte(`{"action":"` + name + `"}`)})
		require.NoError(t, err, name)
		assert.Equal(t, name, ev.EventName(), "an event reports the name it is registered under")
	}
}

func TestEventsReportTheirEnvelopeMetadata(t *testing.T) {
	for _, ev := range []events.DomainEvent{
		&events.ContactCreated{WorkspaceID: 7},
		&events.EmailEngagement{Action: events.NameEmailOpened, WorkspaceID: 7},
		&events.EmailDelivery{Action: events.NameEmailBounced, WorkspaceID: 7},
		&events.MarketingConfirmed{WorkspaceID: 7},
		&events.CollectedEvent{WorkspaceID: 7, Action: "page_view"},
	} {
		assert.EqualValues(t, 7, ev.Workspace(), "%T", ev)
		assert.Positive(t, ev.EventVersion(), "%T", ev)
		assert.NotEmpty(t, ev.EventName(), "%T", ev)
		assert.NotEmpty(t, ev.Project().Action, "%T", ev)
	}
}
