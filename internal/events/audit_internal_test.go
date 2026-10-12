package events

import (
	"encoding/json"
	"testing"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
)

func auditMessage(t *testing.T) *message.Message {
	t.Helper()
	data, err := json.Marshal(&AuditEntry{WorkspaceID: 1, Actor: Actor{Kind: ActorUser}, Action: "membership.update", TargetType: "membership"})
	require.NoError(t, err)
	body, err := json.Marshal(Envelope{ID: "evt", Name: NameAuditEntry, Version: 1, WorkspaceID: 1, Data: data})
	require.NoError(t, err)
	return message.NewMessage(watermill.NewUUID(), body)
}

// The automations consumer skips an Audit entry: an administrative action never
// enrolls anyone.
func TestAutomationsConsumerSkipsAuditEntries(t *testing.T) {
	enroller := newEnroller()
	require.NoError(t, automationsConsumer(enroller)(auditMessage(t)))
	assert.Empty(t, enroller.OnEventCalls())
}

// The webhooks consumer hands an Audit entry to the dispatcher under its bus type
// (not the entry's action), so an endpoint selects it as "audit.entry" (ADR 0022).
func TestWebhooksConsumerForwardsAuditEntriesAsAuditEntry(t *testing.T) {
	d := newDispatcher()
	require.NoError(t, webhooksConsumer(&ent.Client{}, d)(auditMessage(t)))
	calls := d.DispatchCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, NameAuditEntry, calls[0].EventName)

	var payload webhookPayload
	require.NoError(t, json.Unmarshal(calls[0].Body, &payload))
	assert.Equal(t, NameAuditEntry, payload.Type)
	assert.JSONEq(t, `{"workspaceId":1,"actor":{"kind":"user"},"action":"membership.update","targetType":"membership"}`, string(payload.Data))
}

// A forwarded entry shows an Operator only as "sphericon staff", like every read surface:
// the staff id never reaches a customer's SIEM (ADR 0022).
func TestWebhooksConsumerMasksTheOperatorIdentity(t *testing.T) {
	data, err := json.Marshal(&AuditEntry{
		WorkspaceID: 1, Actor: Actor{Kind: ActorOperator, ID: "op-42", Name: "Jane Staff"},
		Action: ActionWorkspaceSuspend, TargetType: "workspace",
	})
	require.NoError(t, err)
	body, err := json.Marshal(Envelope{ID: "evt", Name: NameAuditEntry, Version: 1, WorkspaceID: 1, Data: data})
	require.NoError(t, err)

	d := newDispatcher()
	require.NoError(t, webhooksConsumer(&ent.Client{}, d)(message.NewMessage(watermill.NewUUID(), body)))

	var payload webhookPayload
	require.Len(t, d.DispatchCalls(), 1)
	body = d.DispatchCalls()[0].Body
	require.NoError(t, json.Unmarshal(body, &payload))
	var entry struct {
		Actor Actor `json:"actor"`
	}
	require.NoError(t, json.Unmarshal(payload.Data, &entry))
	assert.Equal(t, Actor{Kind: ActorOperator, Name: OperatorLabel}, entry.Actor)
	assert.NotContains(t, string(body), "op-42")
	assert.NotContains(t, string(body), "Jane Staff")
}
