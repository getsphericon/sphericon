package events_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/events"
)

func envelopeOf(name, data string) events.Envelope {
	return events.Envelope{ID: "evt", Name: name, Version: 1, WorkspaceID: 1, Data: json.RawMessage(data)}
}

// The suppression consumer hands the envelope to Suppress, which refuses one it cannot decode
// the same way.
func TestSuppressRejectsUndecodableEnvelopes(t *testing.T) {
	ctx := t.Context()
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
		events.NameSmsConfirmed,
		events.NameSmsDelivered,
		events.NameSmsUndelivered,
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
		&events.SmsConfirmed{WorkspaceID: 7},
		&events.SmsDelivery{Action: events.NameSmsUndelivered, WorkspaceID: 7},
		&events.CollectedEvent{WorkspaceID: 7, Action: "page_view"},
	} {
		assert.EqualValues(t, 7, ev.Workspace(), "%T", ev)
		assert.Positive(t, ev.EventVersion(), "%T", ev)
		assert.NotEmpty(t, ev.EventName(), "%T", ev)
		assert.NotEmpty(t, ev.Project().Action, "%T", ev)
	}
}
