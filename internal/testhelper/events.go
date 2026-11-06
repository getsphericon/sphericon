package testhelper

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee"
	"github.com/getsphericon/sphericon/ee/audit"
	"github.com/getsphericon/sphericon/ee/export"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/ingest"
	"github.com/getsphericon/sphericon/internal/jobs"
)

// A domain event reaches its consumers as River jobs inserted by the publish (ADR 0101). Nothing
// works the queue under txdb, so a test reads the jobs a publish inserted (EventJobs) and works
// them through the consumer's real worker (WorkEventJobs).

// QueuedEnvelopes are the events this test's publishes handed to a consumer, one envelope per
// event, in the order they were published; with names, only those of these bus types. A publish
// writes the Event row and inserts the job of every consumer that wants the event, and every live
// event goes at least to the webhooks consumer (jobs' TestEveryLiveEventIsHandedToTheWebhooksConsumer),
// so this is what the test published: a repeat is not, and loaded history is unless a consumer
// wants it (StoredEvents shows those).
func (env *TestEnv) QueuedEnvelopes(t *testing.T, names ...string) []events.Envelope {
	t.Helper()
	return queuedEnvelopes(t, env.riverClient(t), names)
}

// QueuedEnvelopesOn is QueuedEnvelopes for a database a test wrote to for real (RealDB).
func QueuedEnvelopesOn(t *testing.T, sqlDB *sql.DB, names ...string) []events.Envelope {
	t.Helper()
	rc, err := river.NewClient(riverdatabasesql.New(sqlDB), &river.Config{})
	require.NoError(t, err)
	return queuedEnvelopes(t, rc, names)
}

func queuedEnvelopes(t *testing.T, rc *river.Client[*sql.Tx], names []string) []events.Envelope {
	t.Helper()
	seen := map[string]bool{}
	var out []events.Envelope
	for _, args := range listArgs(t, rc, river.NewJobListParams().Queues(events.QueueEvents)) {
		e := decodeArgs[events.EventJob](t, args).Envelope
		if seen[e.ID] || (len(names) > 0 && !slices.Contains(names, e.Name)) {
			continue
		}
		seen[e.ID] = true
		out = append(out, e)
	}
	return out
}

// QueuedEvents is QueuedEnvelopes decoded into their typed events.
func (env *TestEnv) QueuedEvents(t *testing.T, names ...string) []events.DomainEvent {
	t.Helper()
	envelopes := env.QueuedEnvelopes(t, names...)
	out := make([]events.DomainEvent, 0, len(envelopes))
	for i := range envelopes {
		ev, err := events.Decode(envelopes[i])
		require.NoError(t, err)
		out = append(out, ev)
	}
	return out
}

// QueuedMessage is one queued event with its payload decoded generically (numbers as
// json.Number, so ids compare exactly).
type QueuedMessage struct {
	Envelope events.Envelope
	Data     map[string]any
}

// QueuedMessages is QueuedEnvelopes with each payload decoded generically.
func (env *TestEnv) QueuedMessages(t *testing.T, names ...string) []QueuedMessage {
	t.Helper()
	envelopes := env.QueuedEnvelopes(t, names...)
	out := make([]QueuedMessage, 0, len(envelopes))
	for i := range envelopes {
		dec := json.NewDecoder(bytes.NewReader(envelopes[i].Data))
		dec.UseNumber()
		var data map[string]any
		require.NoError(t, dec.Decode(&data))
		out = append(out, QueuedMessage{Envelope: envelopes[i], Data: data})
	}
	return out
}

// QueuedCount counts the queued events named name whose payload has every key/value of match
// (values compare by their printed form: 42 matches "42").
func (env *TestEnv) QueuedCount(t *testing.T, name string, match map[string]any) int {
	t.Helper()
	return lo.CountBy(env.QueuedMessages(t, name), func(m QueuedMessage) bool {
		return lo.EveryBy(lo.Keys(match), func(k string) bool { return fmt.Sprint(m.Data[k]) == fmt.Sprint(match[k]) })
	})
}

// FieldChanges is the key and value of each queued contact.field_changed, in the order they were
// published; a cleared field's value is nil.
func (env *TestEnv) FieldChanges(t *testing.T) []map[string]any {
	t.Helper()
	return lo.Map(env.QueuedMessages(t, events.NameContactFieldChanged), func(m QueuedMessage, _ int) map[string]any {
		return map[string]any{"key": m.Data["key"], "value": m.Data["value"]}
	})
}

// StoredEvents are the Event rows of the actions (all of them with none), oldest first: what a
// publish stored, loaded history included.
func (env *TestEnv) StoredEvents(t *testing.T, actions ...string) []*ent.Event {
	t.Helper()
	q := env.DB.Event.Query().Order(event.ByID())
	if len(actions) > 0 {
		q = q.Where(event.ActionIn(actions...))
	}
	rows, err := q.All(t.Context())
	require.NoError(t, err)
	return rows
}

// Publisher is a Publisher onto this test's bus, for a test that calls a function taking the
// Publisher of a transaction it owns (automations.Enroll) without opening one: each event is
// published in a bus transaction of its own, which rides the test's.
func (env *TestEnv) Publisher() events.Publisher { return PublisherOn(env.Bus) }

// PublisherOn is Publisher for a test that holds a bus of its own (one over a real database).
func PublisherOn(bus *events.Bus) events.Publisher { return busPublisher{bus: bus} }

type busPublisher struct{ bus *events.Bus }

func (p busPublisher) Publish(ctx context.Context, ev events.DomainEvent) error {
	return p.bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error { return pub.Publish(ctx, ev) })
}

// BusOn is the application's event bus over a database a test writes to for real (RealDB): a
// publish stores its Event row and inserts the jobs of core's and the Edition's consumers there.
func BusOn(t *testing.T, sqlDB *sql.DB) *events.Bus {
	t.Helper()
	bus := events.New(sqlDB, entclient.NewTx)
	inserter, err := jobs.NewInserter(sqlDB)
	require.NoError(t, err)
	bus.UseJobs(inserter)
	bus.UseConsumers(append(jobs.EventConsumers(), ee.EventConsumers()...)...)
	bus.OnStored(ingest.RecordOutcome)
	return bus
}

// EventJobs are the envelopes of the queued jobs of an event consumer's kind, oldest first: the
// events a publish handed that consumer.
func (env *TestEnv) EventJobs(t *testing.T, kind string) []events.Envelope {
	t.Helper()
	queued := env.JobsOf(t, kind)
	out := make([]events.Envelope, 0, len(queued))
	for _, args := range queued {
		out = append(out, decodeArgs[events.EventJob](t, args).Envelope)
	}
	return out
}

// WorkEventJobs works every queued job of the kind worker works, oldest first, through river's
// test worker with the application's river configuration (Work), so the job runs in the context
// of handling its envelope. Each call works them all again, so calling it twice exercises a job
// river retries.
func WorkEventJobs[T river.JobArgs](t *testing.T, env *TestEnv, worker river.Worker[T]) {
	t.Helper()
	var kind T
	for _, args := range env.JobsOf(t, kind.Kind()) {
		require.NoError(t, Work(t.Context(), t, worker, decodeArgs[T](t, args)), kind.Kind())
	}
}

// WorkOrderJobs works every queued orders job (WorkEventJobs) with the env's Orders module, as
// the orders consumer's worker would. Each call works them all again, so calling it twice
// exercises a redelivered order Event.
func (env *TestEnv) WorkOrderJobs(t *testing.T) {
	t.Helper()
	WorkEventJobs(t, env, jobs.NewOrderEventWorker(env.DB, env.Orders))
}

// WorkEventJobOf works the queued job of the kind worker works that the publish of published
// inserted, and nothing when the consumer's filter left it out.
func WorkEventJobOf[T river.JobArgs](t *testing.T, env *TestEnv, worker river.Worker[T], published events.Envelope) {
	t.Helper()
	var kind T
	for _, args := range env.JobsOf(t, kind.Kind()) {
		if decodeArgs[events.EventJob](t, args).ID == published.ID {
			require.NoError(t, Work(t.Context(), t, worker, decodeArgs[T](t, args)), kind.Kind())
		}
	}
}

// WorkAutomationJobOf works the automations job the publish of published inserted, if any
// (WorkEventJobOf).
func (env *TestEnv) WorkAutomationJobOf(t *testing.T, published events.Envelope) {
	t.Helper()
	WorkEventJobOf(t, env, jobs.NewAutomationEventWorker(env.Inserter(t)), published)
}

// WorkRuleJobOf works the rules job the publish of published inserted, if any (WorkEventJobOf).
func (env *TestEnv) WorkRuleJobOf(t *testing.T, published events.Envelope) {
	t.Helper()
	WorkEventJobOf(t, env, jobs.NewRuleEventWorker(env.Inserter(t)), published)
}

// webhookWorker is the webhooks consumer's worker as the application builds it: its dispatcher
// queues the deliveries in the test's transaction, behind the Edition's license gate.
func (env *TestEnv) webhookWorker(t *testing.T) *jobs.WebhookEventWorker {
	t.Helper()
	return jobs.NewWebhookEventWorker(env.DB, env.edition.Webhooks(jobs.NewWebhookDispatcher(env.Inserter(t))))
}

// WorkWebhookJobs works every queued webhooks job (WorkEventJobs): each queues one delivery per
// enabled endpoint of the event's Workspace whose filter matches.
func (env *TestEnv) WorkWebhookJobs(t *testing.T) {
	t.Helper()
	WorkEventJobs(t, env, env.webhookWorker(t))
}

// WorkWebhookJobOf works the webhooks job the publish of published inserted, if any
// (WorkEventJobOf).
func (env *TestEnv) WorkWebhookJobOf(t *testing.T, published events.Envelope) {
	t.Helper()
	WorkEventJobOf(t, env, env.webhookWorker(t), published)
}

// WorkEditionJobs works every queued job of the Enterprise Edition's consumers, the Audit log's and
// the Erasures stream's (WorkEventJobs), under the Instance's License as the application does.
// Each call works them all again, so calling it twice exercises a retried job.
func (env *TestEnv) WorkEditionJobs(t *testing.T) {
	t.Helper()
	WorkEventJobs(t, env, audit.NewEntryEventWorker(env.DB, env.license))
	WorkEventJobs(t, env, export.NewErasureEventWorker(env.DB, env.license))
}

// QueuedDeliveries are the queued webhook deliveries, oldest first.
func (env *TestEnv) QueuedDeliveries(t *testing.T) []jobs.DeliverWebhookArgs {
	t.Helper()
	queued := env.JobsOf(t, jobs.DeliverWebhookArgs{}.Kind())
	out := make([]jobs.DeliverWebhookArgs, 0, len(queued))
	for _, args := range queued {
		out = append(out, decodeArgs[jobs.DeliverWebhookArgs](t, args))
	}
	return out
}

// Inserter is a river client over the test's connection with the application's configuration,
// for a worker that queues follow-up jobs: what it queues is in the test's transaction, where
// JobsOf reads it.
func (env *TestEnv) Inserter(t *testing.T) *river.Client[*sql.Tx] {
	t.Helper()
	inserter, err := jobs.NewInserter(env.SQLDB)
	require.NoError(t, err)
	return inserter
}

// WorkAutomationJobs works every queued automations job (WorkEventJobs): each queues the
// evaluation of its Event against the Automations (jobs.EvaluateTriggerArgs).
func (env *TestEnv) WorkAutomationJobs(t *testing.T) {
	t.Helper()
	WorkEventJobs(t, env, jobs.NewAutomationEventWorker(env.Inserter(t)))
}

// WorkRuleJobs works every queued rules job (WorkEventJobs): each queues the evaluation of its
// Event against the Rules (jobs.EvaluateRulesArgs).
func (env *TestEnv) WorkRuleJobs(t *testing.T) {
	t.Helper()
	WorkEventJobs(t, env, jobs.NewRuleEventWorker(env.Inserter(t)))
}

// decodeArgs is a queued job's args, read back as JobsOf returns them, decoded into T.
func decodeArgs[T any](t *testing.T, args map[string]any) T {
	t.Helper()
	encoded, err := json.Marshal(args)
	require.NoError(t, err)
	var out T
	require.NoError(t, json.Unmarshal(encoded, &out))
	return out
}
