package events

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rawMsg(payload string) *message.Message {
	return message.NewMessage(watermill.NewUUID(), []byte(payload))
}

func envelopeMsg(t *testing.T, name, data string) *message.Message {
	t.Helper()
	body, err := json.Marshal(Envelope{ID: "evt", Name: name, Version: 1, WorkspaceID: 1, Data: json.RawMessage(data)})
	require.NoError(t, err)
	return rawMsg(string(body))
}

// Every consumer rejects a payload that is not an envelope and an envelope it
// cannot decode, so the router retries or parks the message instead of dropping it
// silently. (The error comes before the consumer touches its dependency, hence nil.)
func TestConsumersRejectUndecodableMessages(t *testing.T) {
	consumers := map[string]message.NoPublishHandlerFunc{
		"persist":     persistConsumer(nil),
		"suppression": suppressionConsumer(nil),
		"webhooks":    webhooksConsumer(nil, nil),
		"automations": automationsConsumer(nil),
		"rules":       rulesConsumer(nil),
	}
	for name, handle := range consumers {
		require.Error(t, handle(rawMsg("not json")), name+": not an envelope")
		require.ErrorContains(t, handle(envelopeMsg(t, "no.such.event", "{}")), "unknown event", name)
		require.ErrorContains(t, handle(envelopeMsg(t, NameContactCreated, `"scalar"`)), "decode", name)
	}
}

func TestDecodeKnowsEveryEventName(t *testing.T) {
	for name := range registry {
		ev, err := Decode(Envelope{Name: name, Data: []byte(`{"action":"` + name + `"}`)})
		require.NoError(t, err, name)
		assert.Equal(t, name, ev.EventName(), "an event reports the name it is registered under")
	}
}

func TestEventsReportTheirEnvelopeMetadata(t *testing.T) {
	for _, ev := range []DomainEvent{
		&ContactCreated{WorkspaceID: 7},
		&EmailEngagement{Action: NameEmailOpened, WorkspaceID: 7},
		&EmailDelivery{Action: NameEmailBounced, WorkspaceID: 7},
		&MarketingConfirmed{WorkspaceID: 7},
		&CollectedEvent{WorkspaceID: 7, Action: "page_view"},
	} {
		assert.EqualValues(t, 7, ev.Workspace(), "%T", ev)
		assert.Positive(t, ev.EventVersion(), "%T", ev)
		assert.NotEmpty(t, ev.EventName(), "%T", ev)
		assert.NotEmpty(t, ev.Project().Action, "%T", ev)
	}
}

// The OTel middleware is transparent: it hands back whatever the handler returned,
// success or failure.
func TestOtelMiddlewarePassesResultsThrough(t *testing.T) {
	boom := errors.New("handler failed")
	out := []*message.Message{rawMsg("out")}

	ok, err := otelMiddleware()(func(*message.Message) ([]*message.Message, error) { return out, nil })(rawMsg("in"))
	require.NoError(t, err)
	assert.Equal(t, out, ok)

	_, err = otelMiddleware()(func(*message.Message) ([]*message.Message, error) { return nil, boom })(rawMsg("in"))
	assert.ErrorIs(t, err, boom)
}
