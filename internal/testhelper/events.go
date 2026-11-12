package testhelper

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertype"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	eejobs "github.com/getsphericon/sphericon/ee/jobs"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/domain/contacts/channelstop"
	"github.com/getsphericon/sphericon/internal/domain/contacts/suppressions"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// A domain event reaches its consumers as River jobs inserted by the publish (ADR 0101). No River
// runtime works the queue under txdb, so a test reads the jobs a publish inserted (EventJobs) and
// works them with their real workers (PerformEnqueuedJobs).

// QueuedEnvelopes are the events this test's publishes handed to a consumer, one envelope per
// event, in the order they were published; with names, only those of these bus types. A publish
// writes the Event row and inserts the job of every consumer that wants the event, and the harness
// registers a consumer of its own that wants every live event (recorder), so this is what the test
// published: a repeat is not, and loaded history is unless a consumer wants it (StoredEvents shows
// those).
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
		if e.ID == "" || seen[e.ID] || (len(names) > 0 && !slices.Contains(names, e.Name)) {
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
	return lo.Map(env.QueuedMessages(t, systemevent.ContactFieldChanged), func(m QueuedMessage, _ int) map[string]any {
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
// Publisher of a transaction it owns (automations.Exit) without opening one: each event is
// published in a bus transaction of its own, which rides the test's.
func (env *TestEnv) Publisher() events.Publisher { return PublisherOn(env.Bus) }

// PublisherOn is Publisher for a test that holds a bus of its own (one over a real database).
func PublisherOn(bus *events.Bus) events.Publisher { return busPublisher{bus: bus} }

type busPublisher struct{ bus *events.Bus }

func (p busPublisher) Publish(ctx context.Context, ev events.DomainEvent) error {
	return p.bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error { return pub.Publish(ctx, ev) })
}

// recordedArgs is the job of the harness's own consumer of the domain events (recorder): every live
// event a test publishes is handed to it, whether or not a consumer of the application wants it,
// so QueuedEnvelopes sees an event no consumer reacts to yet. Working it does nothing.
type recordedArgs struct {
	events.EventJob
}

func (recordedArgs) Kind() string { return "test_event_recorded" }

// recordedWorker works the recorder's jobs, doing nothing.
type recordedWorker struct {
	river.WorkerDefaults[recordedArgs]
}

func (recordedWorker) Work(context.Context, *river.Job[recordedArgs]) error { return nil }

// recorder is the harness's consumer of every live event (recordedArgs).
var recorder = events.Consumer{
	Wants: func(_ events.Envelope, ev events.DomainEvent) bool { return !events.IsBackfill(ev) },
	Job: func(env events.Envelope, _ events.DomainEvent) river.JobArgs {
		return recordedArgs{events.EventJob{Envelope: env}}
	},
}

// BusOn is the application's event bus over a database a test writes to for real (RealDB): a
// publish stores its Event row and inserts the jobs of core's and the Edition's consumers there.
func BusOn(t *testing.T, sqlDB *sql.DB) *events.Bus {
	t.Helper()
	bus := events.New(sqlDB, Clients(t).NewTx, time.Now)
	inserter, err := jobs.NewInserter(sqlDB)
	require.NoError(t, err)
	bus.UseJobs(inserter)
	bus.UseSubscribers(channelstop.Subscribers()...)
	bus.UseSubscribers(suppressions.Subscribers()...)
	bus.UseConsumers(append(jobs.EventConsumers(), eejobs.EventConsumers()...)...)
	bus.UseConsumers(recorder)
	bus.OnStored(jobs.RecordSendOutcome)
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

// PerformEventJobOf works the queued job of args' kind that the publish of published inserted, as
// PerformEnqueuedJobs does, and nothing when the consumer's filter left it out.
func (env *TestEnv) PerformEventJobOf(t *testing.T, args river.JobArgs, published events.Envelope) {
	t.Helper()
	env.performEnqueuedJobsWhere(t, true, func(job *rivertype.JobRow) bool {
		var queued events.EventJob
		require.NoError(t, json.Unmarshal(job.EncodedArgs, &queued))
		return queued.ID == published.ID
	}, args)
}

// RetryJobs makes the completed jobs of the kinds of only, and those waiting for their next
// attempt after a failure, available again with river's own retry (JobRetry), so the next
// PerformEnqueuedJobs works them again, as river redelivers a job.
func (env *TestEnv) RetryJobs(t *testing.T, only ...river.JobArgs) {
	t.Helper()
	require.NotEmpty(t, only)
	kinds := make([]string, len(only))
	for i, args := range only {
		kinds[i] = args.Kind()
	}
	rc := env.riverClient(t)
	res, err := rc.JobList(t.Context(), river.NewJobListParams().Kinds(kinds...).
		States(rivertype.JobStateCompleted, rivertype.JobStateRetryable).First(maxQueuedJobs))
	require.NoError(t, err)
	for _, job := range res.Jobs {
		_, err := rc.JobRetry(t.Context(), job.ID)
		require.NoError(t, err)
	}
}

// EditionJobs are the jobs of the Enterprise Edition's consumers, the Audit log's and the Erasures
// stream's, for PerformEnqueuedJobs.
var EditionJobs = []river.JobArgs{eejobs.EntryEventArgs{}, eejobs.ErasureEventArgs{}}

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

// decodeArgs is a queued job's args, read back as JobsOf returns them, decoded into T.
func decodeArgs[T any](t *testing.T, args map[string]any) T {
	t.Helper()
	encoded, err := json.Marshal(args)
	require.NoError(t, err)
	var out T
	require.NoError(t, json.Unmarshal(encoded, &out))
	return out
}
