package jobs_test

import (
	"encoding/json"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Every live Event and every Audit entry is handed to the webhooks consumer, which finds the
// endpoints that want it; loaded history is not, and neither is an event of the Instance or a
// change told only to a Workspace's people (an Invitation carries its link's token), which no
// endpoint may receive.
func TestEveryLiveEventAndAuditEntryIsHandedToTheWebhooksConsumer(t *testing.T) {
	webhooks, ok := lo.Find(jobs.EventConsumers(), func(c events.Consumer) bool {
		return c.Job(events.Envelope{}, &events.CollectedEvent{}).Kind() == jobs.WebhookEventArgs{}.Kind()
	})
	require.True(t, ok, "the webhooks consumer is registered")

	for _, name := range events.Names() {
		env := events.Envelope{ID: "evt", Name: name, WorkspaceID: fixtures.AcmeID, Data: []byte(`{"action":"` + name + `"}`)}
		ev, err := events.Decode(env)
		require.NoError(t, err, name)
		reachesEndpoints := !events.IsUnprojected(name) || name == events.NameAuditEntry
		handed := webhooks.Wants(env, ev)
		assert.Equal(t, reachesEndpoints, handed, "%s: only an Event or an Audit entry leaves through an endpoint", name)
	}
	history := &events.CollectedEvent{WorkspaceID: fixtures.AcmeID, Action: "purchase", Backfill: true}
	historyWanted := webhooks.Wants(events.Envelope{Name: events.NameCollected}, history)
	assert.False(t, historyWanted, "loaded history is never delivered")
}

// deliveryTo is the queued delivery to the endpoint, failing the test when there is none.
func deliveryTo(t *testing.T, env *testhelper.TestEnv, endpointID int64) jobs.DeliverWebhookArgs {
	t.Helper()
	d, ok := lo.Find(env.QueuedDeliveries(t), func(d jobs.DeliverWebhookArgs) bool { return d.EndpointID == endpointID })
	require.True(t, ok, "a delivery to endpoint %d", endpointID)
	return d
}

// bodyOf is a delivery's body, decoded.
func bodyOf(t *testing.T, d jobs.DeliverWebhookArgs) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(d.Body, &body))
	return body
}

// A customer-facing event inserts one webhooks job; working it queues a delivery to every enabled
// endpoint that selects it, under the semantic action, with the public body: id, type, occurredAt,
// workspaceId, subject, contactId and data, nothing else.
func TestAnEventIsDeliveredToTheEndpointsThatSelectIt(t *testing.T) {
	env := testhelper.Setup(t)
	publishEvent(t, env, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Email: fixtures.ContactAliceEmail})
	published := env.EventJobs(t, jobs.WebhookEventArgs{}.Kind())
	require.Len(t, published, 1)

	env.PerformEnqueuedJobs(t, jobs.WebhookEventArgs{})

	queued := env.QueuedDeliveries(t)
	require.Len(t, queued, 1)
	d := queued[0]
	assert.EqualValues(t, fixtures.WebhookCodebasicsID, d.EndpointID)
	assert.Equal(t, events.NameContactCreated, d.EventName)
	body := bodyOf(t, d)
	assert.ElementsMatch(t, []string{"id", "type", "occurredAt", "workspaceId", "subject", "contactId", "data"}, lo.Keys(body))
	assert.Equal(t, published[0].ID, body["id"])
	assert.Equal(t, events.NameContactCreated, body["type"])
	assert.EqualValues(t, fixtures.AcmeID, body["workspaceId"])
	assert.Equal(t, fixtures.ContactAliceEmail, body["subject"])
	assert.EqualValues(t, fixtures.ContactAliceID, body["contactId"])
}

// A provider's notification is delivered under its own id, which a repeat of it carries too, so a
// receiver can drop the repeat; a repeat is not even published.
func TestAProviderNotificationIsDeliveredUnderItsOwnID(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.WebhookEndpoint.UpdateOneID(fixtures.WebhookCodebasicsID).SetEventTypes([]string{events.NameEmailBounced}).ExecX(t.Context())
	bounce := notice(events.NameEmailBounced, events.BounceKindHard, fixtures.ContactAliceEmail)
	publishEvent(t, env, bounce)
	publishEvent(t, env, bounce)

	env.PerformEnqueuedJobs(t, jobs.WebhookEventArgs{})

	queued := env.QueuedDeliveries(t)
	require.Len(t, queued, 1)
	assert.Equal(t, bounce.DedupID, queued[0].DeliveryID)
	assert.Equal(t, bounce.DedupID, bodyOf(t, queued[0])["id"])
}

// The contact.erased webhook payload carries the customer's subject_id; nothing else
// identifies the person.
func TestTheErasedContactsDeliveryCarriesItsSubject(t *testing.T) {
	env := testhelper.Setup(t)
	publishEvent(t, env, &events.ContactErased{
		WorkspaceID: fixtures.InitechID, SubjectID: "user-9", IdentifierKind: "contact_id", RequesterKind: "user", RequesterID: 4,
	})

	env.PerformEnqueuedJobs(t, jobs.WebhookEventArgs{})

	d := deliveryTo(t, env, fixtures.WebhookInitechID)
	assert.Equal(t, events.NameContactErased, d.EventName)
	data, ok := bodyOf(t, d)["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "user-9", data["subjectId"])
}

// A backfilled event is recorded but inserts no webhooks job; the same event without the flag
// reaches the endpoint.
func TestABackfilledEventReachesNoWebhook(t *testing.T) {
	env := testhelper.Setup(t)
	history := &events.CollectedEvent{WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactErasableID, SubjectID: "s-7", Action: "purchase", Backfill: true}
	publishEvent(t, env, history)
	historyJobs := env.EventJobs(t, jobs.WebhookEventArgs{}.Kind())
	assert.Empty(t, historyJobs)

	live := &events.CollectedEvent{WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactErasableID, SubjectID: "s-7", Action: "purchase"}
	publishEvent(t, env, live)
	env.PerformEnqueuedJobs(t, jobs.WebhookEventArgs{})

	assert.Equal(t, "purchase", deliveryTo(t, env, fixtures.WebhookInitechID).EventName)
}

// An Audit entry is delivered under its bus type, so an endpoint selects it as "audit.entry", and
// only to an endpoint that selects it (ADR 0022). A Staff member appears in it only as "Sphericon
// staff", like on every read surface: the staff id never reaches a customer's SIEM.
func TestAnAuditEntryReachesOnlyTheEndpointThatSelectsItWithTheStaffMasked(t *testing.T) {
	env := testhelper.Setup(t)
	publishEvent(t, env, &events.AuditEntry{
		WorkspaceID: fixtures.InitechID, Actor: events.Actor{Kind: events.ActorStaff, ID: "op-42", Name: "Jane Staff"},
		Action: events.ActionWorkspaceSuspend, TargetType: "workspace",
	})

	env.PerformEnqueuedJobs(t, jobs.WebhookEventArgs{})

	queued := env.QueuedDeliveries(t)
	require.Len(t, queued, 1, "only the endpoint that selects audit.entry receives it")
	d := queued[0]
	assert.EqualValues(t, fixtures.WebhookInitechAuditID, d.EndpointID)
	assert.Equal(t, events.NameAuditEntry, d.EventName)
	assert.Equal(t, events.NameAuditEntry, bodyOf(t, d)["type"])
	var entry struct {
		Data struct {
			Actor events.Actor `json:"actor"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(d.Body, &entry))
	assert.Equal(t, events.Actor{Kind: events.ActorStaff, Name: events.StaffLabel}, entry.Data.Actor)
	body := string(d.Body)
	assert.NotContains(t, body, "op-42")
	assert.NotContains(t, body, "Jane Staff")
}
