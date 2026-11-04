package testhelper

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/events"
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
