package eventrecording_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eventrecording"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// unserializable cannot be marshalled into an event, so publishing it fails.
func unserializable() map[string]any { return map[string]any{"bad": make(chan int)} }

func TestRecordCarriesTheEmailAndPhoneSnapshot(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, newModule(env).Record(context.Background(), env.DB.Scoped(fixtures.AcmeID), []eventrecording.Input{
		{SubjectID: "s-1", Action: "page_view", Email: new("nobody@example.com"), Phone: new("+15550100")},
	}))

	got := queuedCollected(t, env)
	require.Len(t, got, 1)
	assert.Equal(t, "nobody@example.com", got[0].Email)
	assert.Equal(t, "+15550100", got[0].Phone)
}

// Record is all-or-nothing: a later Event that cannot be published rolls back the earlier ones.
func TestRecordIsAtomicAcrossTheBatch(t *testing.T) {
	env := testhelper.Setup(t)

	err := newModule(env).Record(context.Background(), env.DB.Scoped(fixtures.AcmeID), []eventrecording.Input{
		{SubjectID: "ok", Action: "page_view"},
		{SubjectID: "bad", Action: "page_view", Properties: unserializable()},
	})
	require.Error(t, err)
	assert.Empty(t, queuedCollected(t, env), "none of the batch is accepted")
	assert.Zero(t, stored(t, env, "ok"), "none of the batch is recorded")
}

func TestRecordEachIsolatesItems(t *testing.T) {
	env := testhelper.Setup(t)

	errs := newModule(env).RecordEach(context.Background(), env.DB.Scoped(fixtures.AcmeID), []eventrecording.Input{
		{SubjectID: "s-1", Action: "page_view"},
		{SubjectID: "  ", Action: "page_view"},
		{SubjectID: "s-3", Action: ""},
		{SubjectID: "s-4", Action: "page_view", Properties: unserializable()},
		{SubjectID: "s-5", Action: "purchase"},
	})

	require.Len(t, errs.Items, 5)
	require.NoError(t, errs.Items[0].Err)
	require.ErrorIs(t, errs.Items[1].Err, events.ErrRejected, "blank subject id")
	require.ErrorIs(t, errs.Items[2].Err, events.ErrRejected, "blank action")
	require.Error(t, errs.Items[3].Err)
	require.NotErrorIs(t, errs.Items[3].Err, events.ErrRejected)
	require.NoError(t, errs.Items[4].Err, "an earlier failure does not affect later items")

	got := queuedCollected(t, env)
	require.Len(t, got, 2)
	assert.Equal(t, "s-1", got[0].SubjectID)
	assert.Equal(t, "s-5", got[1].SubjectID)
}

func TestRecordEachWithNoInputs(t *testing.T) {
	env := testhelper.Setup(t)
	assert.Empty(t, newModule(env).RecordEach(context.Background(), env.DB.Scoped(fixtures.AcmeID), nil).Items)
}

func TestRecordMarksBackfilledEvents(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, newModule(env).Record(context.Background(), env.DB.Scoped(fixtures.AcmeID), []eventrecording.Input{
		{SubjectID: "s-1", Action: "purchase", Backfill: true, OccurredAt: testNow.AddDate(-1, 0, 0)},
		{SubjectID: "s-2", Action: "purchase"},
	}))

	// History is stored marked and handed to no consumer; the live Event is both.
	backfill := lo.SliceToMap(env.StoredEvents(t, "purchase"), func(e *ent.Event) (string, bool) { return e.SubjectID, e.Backfill })
	assert.True(t, backfill["s-1"])
	assert.False(t, backfill["s-2"])
	got := queuedCollected(t, env)
	require.Len(t, got, 1)
	assert.Equal(t, "s-2", got[0].SubjectID)
	assert.False(t, got[0].Backfill)
}

// A resent Event with the same client id changes nothing once the first one is stored, and
// the id is scoped to the Workspace.
func TestRecordSkipsAnEventItAlreadyStored(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env)
	acme := env.DB.Scoped(fixtures.AcmeID)
	in := eventrecording.Input{SubjectID: "s-1", Action: "purchase", ClientEventID: "order-1"}

	require.NoError(t, m.Record(ctx, acme, []eventrecording.Input{in}))
	require.NoError(t, m.Record(ctx, acme, []eventrecording.Input{in}))

	got := queuedCollected(t, env)
	require.Len(t, got, 1, "the resend is not published again")
	assert.Equal(t, events.DedupKey(events.SourceAPI, fixtures.AcmeID, "order-1"), got[0].DedupID)
	assert.Equal(t, 1, stored(t, env, "s-1"), "the resend is not recorded again")
}

// stored counts the stored Events of a subject.
func stored(t *testing.T, env *testhelper.TestEnv, subject string) int {
	t.Helper()
	n, err := env.DB.Event.Query().Where(event.WorkspaceID(fixtures.AcmeID), event.SubjectID(subject)).Count(t.Context())
	require.NoError(t, err)
	return n
}

func rejected(t *testing.T, err error) string {
	t.Helper()
	require.ErrorIs(t, err, events.ErrRejected)
	return err.Error()
}

// The system's own actions are never a customer's Event, in any case or spelling.
func TestRecordRefusesReservedActions(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env)
	for _, action := range []string{systemevent.EmailOpened, " Email.Clicked", systemevent.MarketingConfirmed, systemevent.ContactCreated, systemevent.TelegramBound, " Telegram.Unbound"} {
		err := m.Record(context.Background(), env.DB.Scoped(fixtures.AcmeID), []eventrecording.Input{{SubjectID: "s-1", Action: action}})
		reason := rejected(t, err)
		assert.Contains(t, reason, "reserved", action)
	}
	assert.Empty(t, queuedCollected(t, env))
}

func TestRecordRefusesALiveEventThatIsHistory(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env)
	acme := env.DB.Scoped(fixtures.AcmeID)

	stale := eventrecording.Input{SubjectID: "s-1", Action: "purchase", OccurredAt: testNow.Add(-liveWindow - time.Hour)}
	err := m.Record(context.Background(), acme, []eventrecording.Input{stale})
	reason := rejected(t, err)
	assert.Contains(t, reason, "backfill")

	require.NoError(t, m.Record(context.Background(), acme, []eventrecording.Input{
		{SubjectID: "s-1", Action: "purchase", OccurredAt: testNow.Add(-liveWindow + time.Hour)},
		{SubjectID: "s-2", Action: "purchase"},
	}), "an event inside the window, or without a time, is live")
	assert.Len(t, queuedCollected(t, env), 2)
}

func TestBackfillNeedsATimeThatHasPassed(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env)
	acme := env.DB.Scoped(fixtures.AcmeID)
	ctx := context.Background()

	errs := m.RecordEach(ctx, acme, []eventrecording.Input{
		{SubjectID: "s-1", Action: "purchase", Backfill: true},
		{SubjectID: "s-2", Action: "purchase", Backfill: true, OccurredAt: testNow.Add(time.Hour)},
		{SubjectID: "s-3", Action: "purchase", Backfill: true, OccurredAt: testNow.Add(-2 * liveWindow)},
	})

	noTime, future := rejected(t, errs.Items[0].Err), rejected(t, errs.Items[1].Err)
	assert.Contains(t, noTime, "needs occurredAt")
	assert.Contains(t, future, "future")
	assert.NoError(t, errs.Items[2].Err, "history older than the live window is exactly what a backfill is for")
}

// The same client id twice in one batch is one Event, recorded and published once: a live one
// would otherwise enroll or deliver twice.
func TestRecordPublishesARepeatedClientEventIDOnceInABatch(t *testing.T) {
	env := testhelper.Setup(t)
	in := eventrecording.Input{SubjectID: "s-1", Action: "purchase", ClientEventID: "order-1"}

	require.NoError(t, newModule(env).Record(context.Background(), env.DB.Scoped(fixtures.AcmeID), []eventrecording.Input{in, in}))

	assert.Len(t, queuedCollected(t, env), 1)
	assert.Equal(t, 1, stored(t, env, "s-1"))
}
