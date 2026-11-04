package testhelper

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/jobs"
)

// A domain event reaches its consumers as River jobs inserted by the publish (ADR 0101). Nothing
// works the queue under txdb, so a test reads the jobs a publish inserted (EventJobs) and works
// them through the consumer's real worker (WorkEventJobs).

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

// BackdateEvent moves the stored Event of a published envelope to at, for a test that needs an
// Event of the past: an event is stored when it is published, at the time its producer gives,
// and most producers give none. occurred_at never changes on a row, so the row is replaced by a
// copy at that time.
func (env *TestEnv) BackdateEvent(t *testing.T, published events.Envelope, at time.Time) {
	t.Helper()
	ctx := t.Context()
	stored, err := env.DB.Event.Query().Where(event.SourceID(published.SourceID())).Only(ctx)
	require.NoError(t, err, "the envelope's Event is stored")
	require.NoError(t, env.DB.Event.DeleteOne(stored).Exec(ctx))
	err = env.DB.Event.Create().
		SetWorkspaceID(stored.WorkspaceID).
		SetNillableSourceID(stored.SourceID).
		SetNillableContactID(stored.ContactID).
		SetNillableVisitorID(stored.VisitorID).
		SetSubjectID(stored.SubjectID).
		SetNillableEmail(stored.Email).
		SetNillablePhone(stored.Phone).
		SetAction(stored.Action).
		SetProperties(stored.Properties).
		SetOccurredAt(at).
		SetMachine(stored.Machine).
		SetCaptured(stored.Captured).
		SetNillableBroadcastID(stored.BroadcastID).
		SetNillableAutomationID(stored.AutomationID).
		SetNillableURL(stored.URL).
		SetBackfill(stored.Backfill).
		Exec(ctx)
	require.NoError(t, err)
}
