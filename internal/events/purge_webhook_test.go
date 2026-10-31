package events_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobkind"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// deliveryBody publishes ev through the real bus and runs the webhooks consumer over the
// stored envelope, so the body is exactly what dispatch hands to a queued delivery.
func deliveryBody(t *testing.T, env *testhelper.TestEnv, ev events.DomainEvent) []byte {
	t.Helper()
	ctx := context.Background()
	before := len(env.OutboxEnvelopes(t))
	require.NoError(t, env.Bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(ctx, ev)
	}))
	envelopes := env.OutboxEnvelopes(t)
	require.Len(t, envelopes, before+1)

	queued := len(env.JobsOf(t, jobkind.DeliverWebhook.Kind))
	require.NoError(t, events.WebhooksConsumer(env.DB, queueingJobs(t, env))(events.Message(envelopes[before])))
	after := env.JobsOf(t, jobkind.DeliverWebhook.Kind)
	require.Len(t, after, queued+1)
	encoded, err := json.Marshal(after[queued])
	require.NoError(t, err)
	var job jobs.DeliverWebhookArgs
	require.NoError(t, json.Unmarshal(encoded, &job))
	return job.Body
}

// withoutPath returns body with the JSON object member at path removed, so a delivery
// built by the real serialization can be reduced to carry one Contact path only.
func withoutPath(t *testing.T, body []byte, path ...string) []byte {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(body, &doc))
	parent := doc
	for _, key := range path[:len(path)-1] {
		child, ok := parent[key].(map[string]any)
		require.True(t, ok, "path %v is not in the body", path)
		parent = child
	}
	_, present := parent[path[len(path)-1]]
	require.True(t, present, "path %v is not in the body", path)
	delete(parent, path[len(path)-1])
	out, err := json.Marshal(doc)
	require.NoError(t, err)
	return out
}

// Queued deliveries are matched by the Contact paths of the body dispatch really builds, so
// a change to the event envelope cannot make an Erasure miss one.
func TestPurgeWebhookJobsMatchesTheBodyDispatchBuilds(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	byID := deliveryBody(t, env, &events.ContactCreated{WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactErasableID})
	byAddress := deliveryBody(t, env, &events.CollectedEvent{WorkspaceID: fixtures.InitechID, Action: "page_view", Email: "Erin@Initech.test"})
	bystander := deliveryBody(t, env, &events.ContactCreated{WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactBystanderID, Email: fixtures.ContactBystanderEmail})
	nobody := deliveryBody(t, env, &events.CollectedEvent{WorkspaceID: fixtures.InitechID, Action: "page_view", VisitorID: "anon-visitor"})
	otherTenant := deliveryBody(t, env, &events.ContactCreated{WorkspaceID: fixtures.GlobexID, ContactID: fixtures.ContactErasableID, Email: fixtures.ContactErasableEmail})

	// Each Contact id path alone must be enough to match, so removing either clause of the
	// purge fails here.
	onlyTopLevelID := withoutPath(t, byID, "data", "contactId")
	onlyDataID := withoutPath(t, byID, "contactId")

	// The paths the purge reads are in the body: the Contact id at its top level and under
	// the data, the address under the data.
	var payload struct {
		ContactID int64 `json:"contactId"`
		Data      struct {
			ContactID int64  `json:"contactId"`
			Email     string `json:"email"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(bystander, &payload))
	assert.EqualValues(t, fixtures.ContactBystanderID, payload.ContactID)
	assert.EqualValues(t, fixtures.ContactBystanderID, payload.Data.ContactID)
	assert.Equal(t, fixtures.ContactBystanderEmail, payload.Data.Email)

	names := make([]string, 0, 7)
	for _, d := range []struct {
		endpoint int64
		id       string
		body     []byte
	}{
		{fixtures.WebhookInitechID, "by-id", byID},
		{fixtures.WebhookInitechID, "by-address", byAddress},
		{fixtures.WebhookInitechID, "by-top-level-id-only", onlyTopLevelID},
		{fixtures.WebhookInitechID, "by-data-id-only", onlyDataID},
		{fixtures.WebhookInitechID, "bystander", bystander},
		{fixtures.WebhookInitechID, "nobody", nobody},
		{fixtures.WebhookGlobexID, "other-tenant", otherTenant},
	} {
		names = append(names, d.id)
		env.EnqueueJob(t, jobs.DeliverWebhookArgs{EndpointID: d.endpoint, EventName: "x", DeliveryID: d.id, Body: d.body})
	}

	scope := env.DB.Scoped(fixtures.InitechID)
	require.NoError(t, env.Bus.WithinScopedPurgeTx(ctx, scope, func(_ *ent.Scoped, pub events.PurgingPublisher) error {
		return pub.PurgeWebhookJobs(ctx, fixtures.InitechID, fixtures.ContactErasableID, []string{fixtures.ContactErasableEmail})
	}))

	// The deliveries queued while building the bodies above stay in the queue too; only the
	// ones enqueued here under a name are the purge's subject.
	queuedIDs := lo.Map(env.JobsOf(t, jobkind.DeliverWebhook.Kind), func(args map[string]any, _ int) string {
		return fmt.Sprint(args["delivery_id"])
	})
	left := lo.Intersect(queuedIDs, names)
	assert.ElementsMatch(t, []string{"bystander", "nobody", "other-tenant"}, left)
}
