package events_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/event"
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
	body, err := json.Marshal(events.Envelope{ID: "evt_test", Name: ev.EventName(), Version: ev.EventVersion(), WorkspaceID: ev.Workspace(), OccurredAt: occurred, Data: data})
	require.NoError(t, err)
	return message.NewMessage(watermill.NewUUID(), body)
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

// The automations consumer enrolls the contact on the event's semantic action.
func TestAutomationsConsumerEnrollsContact(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, events.AutomationsConsumer(queueingJobs(t, env))(msgFor(t, &events.EmailEngagement{
		Action: events.NameEmailOpened, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Email: fixtures.ContactAliceEmail,
	})))

	queued := env.JobsOf(t, jobkind.EvaluateTrigger.Kind)
	require.Len(t, queued, 1)
	assert.EqualValues(t, fixtures.AcmeID, queued[0]["workspace_id"])
	assert.EqualValues(t, fixtures.ContactAliceID, queued[0]["contact_id"])
	assert.Equal(t, events.NameEmailOpened, queued[0]["action"])
}

// The Event's row is stored before its evaluation is queued, so the job that ends or makes an
// Enrollment finds the Event among the Contact's whichever consumer runs first (#751); the persist
// consumer's write of the same envelope, or a redelivery, stores no second row.
func TestAutomationsConsumerStoresTheEventBeforeItQueuesTheJob(t *testing.T) {
	env := testhelper.Setup(t)
	consume := events.AutomationsConsumer(queueingJobs(t, env))
	paid := &events.CollectedEvent{
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Action: "payment_succeeded", OccurredAt: time.Now(),
	}
	stored := func() int {
		t.Helper()
		n, err := env.DB.Event.Query().Where(event.SourceID("evt_test")).Count(t.Context())
		require.NoError(t, err)
		return n
	}

	require.NoError(t, consume(msgFor(t, paid)))
	afterFirst := stored()
	require.NoError(t, consume(msgFor(t, paid)))
	afterSecond := stored()
	queued := env.JobsOf(t, jobkind.EvaluateTrigger.Kind)

	assert.Equal(t, 1, afterFirst, "the row is there once the job is queued")
	assert.Len(t, queued, 2)
	assert.Equal(t, 1, afterSecond, "a second write of the envelope stores no second row")
}

// The goal Event reaches the Automations consumer before the trigger's evaluation has run: its own
// evaluation finds no Enrollment to end, but its row is stored, so the trigger's evaluation then
// makes the Enrollment already at its goal, without waiting for the persist consumer.
func TestAGoalEventConsumedBeforeTheTriggerEndsTheEnrollmentWhenItIsMade(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	failed := time.Now().Add(-time.Hour)
	paid := failed.Add(30 * time.Minute)
	require.NoError(t, events.AutomationsConsumer(queueingJobs(t, env))(msgAt(t, &events.CollectedEvent{
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Action: "payment_succeeded", OccurredAt: paid,
	}, paid)))

	require.NoError(t, stepping.OnEvent(ctx, env.Bus, acme, fixtures.ContactAliceID, automations.Event{Action: "payment_succeeded", OccurredAt: paid}, 0))
	require.NoError(t, stepping.OnEvent(ctx, env.Bus, acme, fixtures.ContactAliceID, automations.Event{Action: "payment_failed", OccurredAt: failed}, 0))

	run, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationPaymentRecoveryID), automationrun.ContactID(fixtures.ContactAliceID)).
		Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusGoalReached, run.Status)
}

// When the Event occurred travels with the job: an Event older than an Enrollment ends nothing
// (#751).
func TestAutomationsConsumerSaysWhenTheEventOccurred(t *testing.T) {
	env := testhelper.Setup(t)
	occurred := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	require.NoError(t, events.AutomationsConsumer(queueingJobs(t, env))(msgAt(t, &events.CollectedEvent{
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Action: "payment_succeeded", OccurredAt: occurred,
	}, occurred)))

	queued := env.JobsOf(t, jobkind.EvaluateTrigger.Kind)
	require.Len(t, queued, 1)
	assert.Equal(t, "2026-03-01T12:00:00Z", queued[0]["occurred_at"])
}

// The Automation whose email the Event came from travels with the job, so it can refuse to
// enroll the Contact into itself (ADR 0053).
func TestAutomationsConsumerNamesTheAutomationOfTheEmail(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, events.AutomationsConsumer(queueingJobs(t, env))(msgFor(t, &events.EmailEngagement{
		Action: events.NameEmailOpened, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID,
		Email: fixtures.ContactAliceEmail, AutomationID: fixtures.AutomationOnOpenID,
	})))

	queued := env.JobsOf(t, jobkind.EvaluateTrigger.Kind)
	require.Len(t, queued, 1)
	assert.EqualValues(t, fixtures.AutomationOnOpenID, queued[0]["source_automation_id"])
}

// A machine open or click is recorded but enrolls nobody (ADR 0053).
func TestAutomationsConsumerSkipsMachineEngagement(t *testing.T) {
	env := testhelper.Setup(t)
	enroll := events.AutomationsConsumer(queueingJobs(t, env))

	for _, action := range []string{events.NameEmailOpened, events.NameEmailClicked} {
		require.NoError(t, enroll(msgFor(t, &events.EmailEngagement{
			Action: action, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID,
			Email: fixtures.ContactAliceEmail, Machine: true,
		})))
	}
	assert.Empty(t, env.JobsOf(t, jobkind.EvaluateTrigger.Kind))

	require.NoError(t, enroll(msgFor(t, &events.EmailEngagement{
		Action: events.NameEmailOpened, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID,
		Email: fixtures.ContactAliceEmail,
	})))
	assert.Len(t, env.JobsOf(t, jobkind.EvaluateTrigger.Kind), 1, "a person's open still enrolls")
}

// Collected (customer) events carry no contact, so they are not enrolled.
func TestAutomationsConsumerSkipsContactlessEvents(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, events.AutomationsConsumer(queueingJobs(t, env))(msgFor(t, &events.CollectedEvent{
		WorkspaceID: fixtures.AcmeID, SubjectID: "visitor:x", Action: "page_view",
	})))

	assert.Empty(t, env.JobsOf(t, jobkind.EvaluateTrigger.Kind))
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

// A backfilled event is recorded but neither enrolls its contact nor reaches a webhook
// endpoint; the same event without the flag does both.
func TestBackfilledEventsTriggerNeitherAutomationsNorWebhooks(t *testing.T) {
	env := testhelper.Setup(t)
	client := queueingJobs(t, env)
	enroll, deliver := events.AutomationsConsumer(client), events.WebhooksConsumer(env.DB, client)

	history := &events.CollectedEvent{WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactErasableID, SubjectID: "s-7", Action: "purchase", Backfill: true}
	require.NoError(t, enroll(msgFor(t, history)))
	require.NoError(t, deliver(msgFor(t, history)))
	assert.Empty(t, env.JobsOf(t, jobkind.EvaluateTrigger.Kind))
	assert.Empty(t, env.JobsOf(t, jobkind.DeliverWebhook.Kind))

	live := &events.CollectedEvent{WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactErasableID, SubjectID: "s-7", Action: "purchase"}
	require.NoError(t, enroll(msgFor(t, live)))
	require.NoError(t, deliver(msgFor(t, live)))
	assert.Len(t, env.JobsOf(t, jobkind.EvaluateTrigger.Kind), 1)
	assert.Contains(t, deliveries(t, env), int64(fixtures.WebhookInitechID))
}

// The automations consumer skips an Audit entry: an administrative action never
// enrolls anyone.
func TestAutomationsConsumerSkipsAuditEntries(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, events.AutomationsConsumer(queueingJobs(t, env))(msgFor(t, entryBy(events.Actor{Kind: events.ActorUser}, "membership.update"))))

	assert.Empty(t, env.JobsOf(t, jobkind.EvaluateTrigger.Kind))
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
