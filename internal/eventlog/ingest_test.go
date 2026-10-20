package eventlog_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/eventlog"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// unserializable cannot be marshalled into the outbox payload, so publishing it fails.
func unserializable() map[string]any { return map[string]any{"bad": make(chan int)} }

func TestIngestCarriesTheEmailAndPhoneSnapshot(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, newModule(env).Ingest(context.Background(), env.DB.Scoped(fixtures.AcmeID), []eventlog.Input{
		{SubjectID: "s-1", Action: "page_view", Email: ptr("nobody@example.com"), Phone: ptr("+15550100")},
	}))

	got := outboxCollected(t, env)
	require.Len(t, got, 1)
	assert.Equal(t, "nobody@example.com", got[0].Email)
	assert.Equal(t, "+15550100", got[0].Phone)
}

// Ingest is all-or-nothing: a later Event that cannot be published rolls back the earlier ones.
func TestIngestIsAtomicAcrossTheBatch(t *testing.T) {
	env := testhelper.Setup(t)

	err := newModule(env).Ingest(context.Background(), env.DB.Scoped(fixtures.AcmeID), []eventlog.Input{
		{SubjectID: "ok", Action: "page_view"},
		{SubjectID: "bad", Action: "page_view", Properties: unserializable()},
	})
	require.Error(t, err)
	assert.Empty(t, outboxCollected(t, env), "none of the batch is accepted")
}

func TestIngestEachIsolatesItems(t *testing.T) {
	env := testhelper.Setup(t)

	errs := newModule(env).IngestEach(context.Background(), env.DB.Scoped(fixtures.AcmeID), []eventlog.Input{
		{SubjectID: "s-1", Action: "page_view"},
		{SubjectID: "  ", Action: "page_view"},
		{SubjectID: "s-3", Action: ""},
		{SubjectID: "s-4", Action: "page_view", Properties: unserializable()},
		{SubjectID: "s-5", Action: "purchase"},
	})

	require.Len(t, errs, 5)
	assert.NoError(t, errs[0])
	var rejection *events.Rejection
	assert.ErrorAs(t, errs[1], &rejection, "blank subject id")
	assert.ErrorAs(t, errs[2], &rejection, "blank action")
	assert.Error(t, errs[3])
	assert.NotErrorAs(t, errs[3], &rejection)
	assert.NoError(t, errs[4], "an earlier failure does not affect later items")

	got := outboxCollected(t, env)
	require.Len(t, got, 2)
	assert.Equal(t, "s-1", got[0].SubjectID)
	assert.Equal(t, "s-5", got[1].SubjectID)
}

func TestIngestEachWithNoInputs(t *testing.T) {
	env := testhelper.Setup(t)
	assert.Empty(t, newModule(env).IngestEach(context.Background(), env.DB.Scoped(fixtures.AcmeID), nil))
}

func TestIngestMarksBackfilledEvents(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, newModule(env).Ingest(context.Background(), env.DB.Scoped(fixtures.AcmeID), []eventlog.Input{
		{SubjectID: "s-1", Action: "purchase", Backfill: true, OccurredAt: testNow.AddDate(-1, 0, 0)},
		{SubjectID: "s-2", Action: "purchase"},
	}))

	got := outboxCollected(t, env)
	require.Len(t, got, 2)
	assert.True(t, got[0].Backfill)
	assert.False(t, got[1].Backfill)
}

// A resent Event with the same client id changes nothing once the first one is stored, and
// the id is scoped to the Workspace.
func TestIngestSkipsAnEventItAlreadyStored(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env)
	acme := env.DB.Scoped(fixtures.AcmeID)
	in := eventlog.Input{SubjectID: "s-1", Action: "purchase", ClientEventID: "order-1"}

	require.NoError(t, m.Ingest(ctx, acme, []eventlog.Input{in}))
	for _, envelope := range env.OutboxEnvelopes(t, events.NameCollected) {
		require.NoError(t, events.Persist(ctx, env.DB, envelope))
	}
	require.NoError(t, m.Ingest(ctx, acme, []eventlog.Input{in}))

	got := outboxCollected(t, env)
	require.Len(t, got, 1, "the resend is not published again")
	assert.Equal(t, eventlog.DedupKey(fixtures.AcmeID, "order-1"), got[0].DedupID)
}

func rejected(t *testing.T, err error) string {
	t.Helper()
	var rejection *events.Rejection
	require.ErrorAs(t, err, &rejection)
	return rejection.Reason
}

// The system's own actions are never a customer's Event, in any case or spelling.
func TestIngestRefusesReservedActions(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env)
	for _, action := range []string{"email.opened", " Email.Clicked", "marketing.confirmed", "contact.created"} {
		err := m.Ingest(context.Background(), env.DB.Scoped(fixtures.AcmeID), []eventlog.Input{{SubjectID: "s-1", Action: action}})
		reason := rejected(t, err)
		assert.Contains(t, reason, "reserved", action)
	}
	assert.Empty(t, outboxCollected(t, env))
}

func TestIngestRefusesALiveEventThatIsHistory(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env)
	acme := env.DB.Scoped(fixtures.AcmeID)

	stale := eventlog.Input{SubjectID: "s-1", Action: "purchase", OccurredAt: testNow.Add(-liveWindow - time.Hour)}
	err := m.Ingest(context.Background(), acme, []eventlog.Input{stale})
	reason := rejected(t, err)
	assert.Contains(t, reason, "backfill")

	require.NoError(t, m.Ingest(context.Background(), acme, []eventlog.Input{
		{SubjectID: "s-1", Action: "purchase", OccurredAt: testNow.Add(-liveWindow + time.Hour)},
		{SubjectID: "s-2", Action: "purchase"},
	}), "an event inside the window, or without a time, is live")
	assert.Len(t, outboxCollected(t, env), 2)
}

func TestBackfillNeedsATimeThatHasPassed(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env)
	acme := env.DB.Scoped(fixtures.AcmeID)
	ctx := context.Background()

	errs := m.IngestEach(ctx, acme, []eventlog.Input{
		{SubjectID: "s-1", Action: "purchase", Backfill: true},
		{SubjectID: "s-2", Action: "purchase", Backfill: true, OccurredAt: testNow.Add(time.Hour)},
		{SubjectID: "s-3", Action: "purchase", Backfill: true, OccurredAt: testNow.Add(-2 * liveWindow)},
	})

	noTime, future := rejected(t, errs[0]), rejected(t, errs[1])
	assert.Contains(t, noTime, "needs occurredAt")
	assert.Contains(t, future, "future")
	assert.NoError(t, errs[2], "history older than the live window is exactly what a backfill is for")
}

// The same client id twice in one batch is one Event: a live one would otherwise enroll or
// deliver twice before the persist consumer ever sees the repeat.
func TestIngestPublishesARepeatedClientEventIDOnceInABatch(t *testing.T) {
	env := testhelper.Setup(t)
	in := eventlog.Input{SubjectID: "s-1", Action: "purchase", ClientEventID: "order-1"}

	require.NoError(t, newModule(env).Ingest(context.Background(), env.DB.Scoped(fixtures.AcmeID), []eventlog.Input{in, in}))

	assert.Len(t, outboxCollected(t, env), 1)
}
