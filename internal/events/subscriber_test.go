package events

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
)

func newEnroller() *EnrollerMock {
	return &EnrollerMock{OnEventFunc: func(context.Context, int64, int64, string) error { return nil }}
}

func newDispatcher() *WebhookDispatcherMock {
	return &WebhookDispatcherMock{DispatchFunc: func(context.Context, *ent.Scoped, string, string, []byte) error { return nil }}
}

// msgFor builds the watermill message the router would deliver for a typed event.
func msgFor(t *testing.T, ev DomainEvent) *message.Message {
	t.Helper()
	data, err := json.Marshal(ev)
	require.NoError(t, err)
	env := Envelope{ID: "evt_test", Name: ev.EventName(), Version: ev.EventVersion(), WorkspaceID: ev.Workspace(), Data: data}
	body, err := json.Marshal(env)
	require.NoError(t, err)
	return message.NewMessage(watermill.NewUUID(), body)
}

// The automations consumer enrolls the contact on the event's semantic action.
func TestAutomationsConsumerEnrollsContact(t *testing.T) {
	enroller := newEnroller()
	handler := automationsConsumer(enroller)

	require.NoError(t, handler(msgFor(t, &EmailEngagement{
		Action: NameEmailOpened, WorkspaceID: 1, ContactID: 7, Email: "a@b.c",
	})))

	calls := enroller.OnEventCalls()
	require.Len(t, calls, 1)
	assert.EqualValues(t, 1, calls[0].WorkspaceID)
	assert.EqualValues(t, 7, calls[0].ContactID)
	assert.Equal(t, NameEmailOpened, calls[0].Action)
}

// Collected (customer) events carry no contact, so they are not enrolled.
func TestAutomationsConsumerSkipsContactlessEvents(t *testing.T) {
	enroller := newEnroller()
	handler := automationsConsumer(enroller)

	require.NoError(t, handler(msgFor(t, &CollectedEvent{
		WorkspaceID: 1, SubjectID: "visitor:x", Action: "page_view",
	})))

	assert.Empty(t, enroller.OnEventCalls())
}

// The webhooks consumer dispatches with the semantic action and a payload built
// from the event's projection.
func TestWebhooksConsumerBuildsPayload(t *testing.T) {
	d := newDispatcher()
	handler := webhooksConsumer(nil, d)

	require.NoError(t, handler(msgFor(t, &ContactCreated{WorkspaceID: 1, ContactID: 5, Email: "a@b.c"})))

	require.Len(t, d.DispatchCalls(), 1)
	c := d.DispatchCalls()[0]
	assert.EqualValues(t, 1, c.S.WorkspaceID())
	assert.Equal(t, NameContactCreated, c.EventName)

	var p map[string]any
	require.NoError(t, json.Unmarshal(c.Body, &p))
	assert.Equal(t, NameContactCreated, p["type"])
	assert.Equal(t, "a@b.c", p["subject"])
}

// The contact.erased webhook payload carries the customer's subject_id; nothing else
// identifies the person.
func TestWebhooksConsumerCarriesTheErasedSubject(t *testing.T) {
	d := newDispatcher()
	handler := webhooksConsumer(nil, d)

	require.NoError(t, handler(msgFor(t, &ContactErased{WorkspaceID: 1, SubjectID: "user-9", IdentifierKind: "contact_id", StaffKind: "user", StaffID: 4})))

	require.Len(t, d.DispatchCalls(), 1)
	assert.Equal(t, NameContactErased, d.DispatchCalls()[0].EventName)
	var p struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(d.DispatchCalls()[0].Body, &p))
	assert.Equal(t, NameContactErased, p.Type)
	assert.Equal(t, "user-9", p.Data["subjectId"])
}
