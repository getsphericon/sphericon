package events_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobkind"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/messaging/registry"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// queueingJobs is the real jobs client over the test's connection: its OnEvent and Dispatch
// insert the river jobs a worker would run, so a test reads what a consumer left in the queue.
func queueingJobs(t *testing.T, env *testhelper.TestEnv) *jobs.Client {
	t.Helper()
	cfg, err := config.Load("test")
	require.NoError(t, err)
	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	require.NoError(t, err)
	mod := outbound.New(env.Bus, env.Holds, nil, tracking.New("test-secret", "http://local"))
	client, err := jobs.NewClient(riverdatabasesql.New(env.SQLDB), env.DB, mod, cipher, env.Bus, env.SystemMail, nil, nil, registry.Default(), cfg.AppURL, cfg.OutboundAllow, nil, jobs.Retention{})
	require.NoError(t, err)
	return client
}

// msgFor builds the watermill message the router would deliver for a typed event.
func msgFor(t *testing.T, ev events.DomainEvent) *message.Message {
	t.Helper()
	return msgAt(t, ev, time.Time{})
}

// msgAt is msgFor for an event that occurred at the moment.
func msgAt(t *testing.T, ev events.DomainEvent, occurred time.Time) *message.Message {
	t.Helper()
	data, err := json.Marshal(ev)
	require.NoError(t, err)
	return events.Message(events.Envelope{ID: "evt_test", Name: ev.EventName(), Version: ev.EventVersion(), WorkspaceID: ev.Workspace(), OccurredAt: occurred, Data: data})
}

// delivery is a queued webhook delivery with its body decoded.
type delivery struct {
	EventName string
	Raw       []byte
	Body      struct {
		Type    string          `json:"type"`
		Subject string          `json:"subject"`
		Data    json.RawMessage `json:"data"`
	}
}

// deliveries returns the queued webhook deliveries by endpoint id.
func deliveries(t *testing.T, env *testhelper.TestEnv) map[int64]delivery {
	t.Helper()
	out := map[int64]delivery{}
	for _, args := range env.JobsOf(t, jobkind.DeliverWebhook.Kind) {
		raw, err := json.Marshal(args)
		require.NoError(t, err)
		var job jobs.DeliverWebhookArgs
		require.NoError(t, json.Unmarshal(raw, &job))
		d := delivery{EventName: job.EventName, Raw: job.Body}
		require.NoError(t, json.Unmarshal(job.Body, &d.Body))
		out[job.EndpointID] = d
	}
	return out
}

func entryBy(actor events.Actor, action string) *events.AuditEntry {
	return &events.AuditEntry{WorkspaceID: fixtures.InitechID, Actor: actor, Action: action, TargetType: "membership"}
}

// The goal Event is evaluated before the trigger's evaluation has run: its own evaluation finds no
// Enrollment to end, but its row was stored when it was published, so the trigger's evaluation
// then makes the Enrollment already at its goal (#751).
func TestAGoalEventConsumedBeforeTheTriggerEndsTheEnrollmentWhenItIsMade(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	failed := time.Now().Add(-time.Hour)
	paid := failed.Add(30 * time.Minute)
	publishAlone(t, env, &events.CollectedEvent{
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Action: "payment_succeeded", OccurredAt: paid,
	})

	require.NoError(t, stepping.OnEvent(ctx, env.Bus, acme, fixtures.ContactAliceID, automations.Event{Action: "payment_succeeded", OccurredAt: paid}, 0))
	require.NoError(t, stepping.OnEvent(ctx, env.Bus, acme, fixtures.ContactAliceID, automations.Event{Action: "payment_failed", OccurredAt: failed}, 0))

	run, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationPaymentRecoveryID), automationrun.ContactID(fixtures.ContactAliceID)).
		Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusGoalReached, run.Status)
}

// The webhooks consumer queues a delivery with the semantic action and a payload built
// from the event's projection.
func TestWebhooksConsumerBuildsPayload(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, events.WebhooksConsumer(env.DB, queueingJobs(t, env))(msgFor(t, &events.ContactCreated{
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Email: fixtures.ContactAliceEmail,
	})))

	queued := deliveries(t, env)
	require.Len(t, queued, 1)
	d := queued[fixtures.WebhookCodebasicsID]
	assert.Equal(t, events.NameContactCreated, d.EventName)
	assert.Equal(t, events.NameContactCreated, d.Body.Type)
	assert.Equal(t, fixtures.ContactAliceEmail, d.Body.Subject)
}

// The contact.erased webhook payload carries the customer's subject_id; nothing else
// identifies the person.
func TestWebhooksConsumerCarriesTheErasedSubject(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, events.WebhooksConsumer(env.DB, queueingJobs(t, env))(msgFor(t, &events.ContactErased{
		WorkspaceID: fixtures.InitechID, SubjectID: "user-9", IdentifierKind: "contact_id", RequesterKind: "user", RequesterID: 4,
	})))

	d := deliveries(t, env)[fixtures.WebhookInitechID]
	assert.Equal(t, events.NameContactErased, d.EventName)
	assert.Equal(t, events.NameContactErased, d.Body.Type)
	var data map[string]any
	require.NoError(t, json.Unmarshal(d.Body.Data, &data))
	assert.Equal(t, "user-9", data["subjectId"])
}

// A backfilled event is recorded but reaches no webhook endpoint; the same event without the flag
// does.
func TestBackfilledEventsReachNoWebhook(t *testing.T) {
	env := testhelper.Setup(t)
	deliver := events.WebhooksConsumer(env.DB, queueingJobs(t, env))

	history := &events.CollectedEvent{WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactErasableID, SubjectID: "s-7", Action: "purchase", Backfill: true}
	require.NoError(t, deliver(msgFor(t, history)))
	assert.Empty(t, env.JobsOf(t, jobkind.DeliverWebhook.Kind))

	live := &events.CollectedEvent{WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactErasableID, SubjectID: "s-7", Action: "purchase"}
	require.NoError(t, deliver(msgFor(t, live)))
	assert.Contains(t, deliveries(t, env), int64(fixtures.WebhookInitechID))
}

// The webhooks consumer hands an Audit entry on under its bus type (not the entry's
// action), so an endpoint selects it as "audit.entry" (ADR 0022).
func TestWebhooksConsumerForwardsAuditEntriesAsAuditEntry(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, events.WebhooksConsumer(env.DB, queueingJobs(t, env))(msgFor(t, entryBy(events.Actor{Kind: events.ActorUser}, "membership.update"))))

	queued := deliveries(t, env)
	require.Len(t, queued, 1, "only the endpoint that selects audit.entry receives it")
	d := queued[fixtures.WebhookInitechAuditID]
	assert.Equal(t, events.NameAuditEntry, d.EventName)
	assert.Equal(t, events.NameAuditEntry, d.Body.Type)
	assert.JSONEq(t, `{"workspaceId":3,"actor":{"kind":"user"},"action":"membership.update","targetType":"membership"}`, string(d.Body.Data))
}

// A forwarded entry shows a Staff member only as "Sphericon staff", like every read surface:
// the staff id never reaches a customer's SIEM (ADR 0022).
func TestWebhooksConsumerMasksTheStaffIdentity(t *testing.T) {
	env := testhelper.Setup(t)
	staff := events.Actor{Kind: events.ActorStaff, ID: "op-42", Name: "Jane Staff"}

	require.NoError(t, events.WebhooksConsumer(env.DB, queueingJobs(t, env))(msgFor(t, entryBy(staff, events.ActionWorkspaceSuspend))))

	d := deliveries(t, env)[fixtures.WebhookInitechAuditID]
	var entry struct {
		Actor events.Actor `json:"actor"`
	}
	require.NoError(t, json.Unmarshal(d.Body.Data, &entry))
	assert.Equal(t, events.Actor{Kind: events.ActorStaff, Name: events.StaffLabel}, entry.Actor)
	assert.NotContains(t, string(d.Raw), "op-42")
	assert.NotContains(t, string(d.Raw), "Jane Staff")
}
