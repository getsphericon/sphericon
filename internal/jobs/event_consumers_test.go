package jobs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// publishEvent publishes ev in a bus transaction of its own.
func publishEvent(t *testing.T, env *testhelper.TestEnv, ev events.DomainEvent) {
	t.Helper()
	require.NoError(t, env.Bus.WithinTx(t.Context(), func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(t.Context(), ev)
	}))
}

// notice is a provider's Delivery notice of email in Acme.
func notice(action, bounceKind, email string) *events.EmailDelivery {
	return &events.EmailDelivery{
		Action: action, WorkspaceID: fixtures.AcmeID, Email: email, BounceKind: bounceKind, DedupID: action + "/" + email,
	}
}

// A hard bounce or a complaint inserts one suppression job, which carries the whole envelope; a
// soft bounce or a delivery inserts none, and neither does a hard bounce whose transaction rolls
// back.
func TestABounceOrAComplaintQueuesOneSuppressionJob(t *testing.T) {
	env := testhelper.Setup(t)
	publishEvent(t, env, notice(events.NameEmailBounced, events.BounceKindHard, "hard@example.com"))
	publishEvent(t, env, notice(events.NameEmailComplained, "", "complained@example.com"))
	publishEvent(t, env, notice(events.NameEmailBounced, events.BounceKindSoft, "soft@example.com"))
	publishEvent(t, env, notice(events.NameEmailDelivered, "", "delivered@example.com"))
	boom := errors.New("boom")
	err := env.Bus.WithinTx(t.Context(), func(_ *ent.Client, pub events.Publisher) error {
		if err := pub.Publish(t.Context(), notice(events.NameEmailBounced, events.BounceKindHard, "rolled-back@example.com")); err != nil {
			return err
		}
		return boom
	})
	require.ErrorIs(t, err, boom)

	queued := env.EventJobs(t, jobs.SuppressEventArgs{}.Kind())

	assert.Equal(t, []string{"email.bounced/hard@example.com", "email.complained/complained@example.com"},
		lo.Map(queued, func(e events.Envelope, _ int) string { return e.DedupKey }))
	for _, e := range queued {
		assert.NotEmpty(t, e.ID)
		assert.EqualValues(t, fixtures.AcmeID, e.WorkspaceID)
		assert.NotEmpty(t, e.CorrelationID, "the job carries the event's chain")
		_, err := events.Decode(e)
		assert.NoError(t, err, "the job carries the typed event")
	}
}

// Working the suppression job suppresses the address once: a complaint after the bounce, and the
// job worked again as river retries it, keep the first entry and its reason.
func TestTheSuppressionJobSuppressesTheAddressOnce(t *testing.T) {
	env := testhelper.Setup(t)
	publishEvent(t, env, notice(events.NameEmailBounced, events.BounceKindHard, "twice@example.com"))
	publishEvent(t, env, notice(events.NameEmailComplained, "", "twice@example.com"))

	testhelper.WorkEventJobs(t, env, jobs.NewSuppressEventWorker(env.DB))
	testhelper.WorkEventJobs(t, env, jobs.NewSuppressEventWorker(env.DB))

	entries, err := env.DB.Suppression.Query().
		Where(entsuppression.WorkspaceID(fixtures.AcmeID), entsuppression.Destination("twice@example.com")).All(t.Context())
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, entsuppression.ReasonBounce, entries[0].Reason)
}

// A suppression job that fails on its last attempt is discarded, not dropped: it stays for an
// operator to fix the cause and retry it (jobs.DiscardedJobRetention).
func TestASuppressionJobPastItsAttemptsIsDiscarded(t *testing.T) {
	ctx := context.Background()
	sqlDB := testhelper.RealDB(t)
	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	w := rivertest.NewWorker(t, riverdatabasesql.New(sqlDB), jobs.BaseConfig(), river.Worker[jobs.SuppressEventArgs](jobs.NewSuppressEventWorker(db.NewEntClient(sqlDB))))
	undecodable := jobs.SuppressEventArgs{
		ID: "01JUNDECODABLE", Name: "no.such.event", WorkspaceID: fixtures.AcmeID, Data: []byte(`{}`),
	}

	res, err := w.Work(ctx, t, tx, undecodable, &river.InsertOpts{Queue: events.QueueEvents, MaxAttempts: 1})

	require.Error(t, err)
	assert.Equal(t, rivertype.JobStateDiscarded, res.Job.State)
	assert.Equal(t, 30*24*time.Hour, jobs.DiscardedJobRetention, "kept for a month")
}

// chainArgs is an event consumer's job of a test: its worker publishes an event.
type chainArgs struct {
	events.EventJob
}

func (chainArgs) Kind() string { return "event_chain_test" }

// An event an event job publishes is caused by the event the job handles and shares its
// correlation id, whatever chain the job was inserted in: the envelope says what is handled.
func TestAnEventPublishedByAnEventJobIsCausedByTheHandledEvent(t *testing.T) {
	ctx := context.Background()
	sqlDB := testhelper.RealDB(t)
	bus := events.New(sqlDB)
	inserter, err := jobs.NewInserter(sqlDB)
	require.NoError(t, err)
	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	handled := events.Envelope{ID: "01JHANDLED", Name: events.NameContactCreated, WorkspaceID: fixtures.AcmeID, CorrelationID: "01JROOT", Data: []byte(`{}`)}
	// The publisher of the handled event ran while handling another: the job's metadata records
	// that chain, one link up.
	publishing := events.Handling(ctx, events.Envelope{ID: "01JPUBLISHER", CorrelationID: "01JROOT"})
	queued, err := inserter.InsertTx(publishing, tx, chainArgs{events.EventJob{Envelope: handled}}, nil)
	require.NoError(t, err)
	w := rivertest.NewWorker(t, riverdatabasesql.New(sqlDB), jobs.BaseConfig(), river.WorkFunc(func(ctx context.Context, _ *river.Job[chainArgs]) error {
		return bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
			return pub.Publish(ctx, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: 7, Email: "caused@example.com"})
		})
	}))

	_, err = w.WorkJob(ctx, t, tx, queued.Job)

	require.NoError(t, err)
	published, err := testhelper.OutboxEnvelopesOn(ctx, db.NewEntClient(sqlDB), events.NameContactCreated)
	require.NoError(t, err)
	require.NotEmpty(t, published)
	last := published[len(published)-1]
	assert.Equal(t, "01JROOT", last.CorrelationID)
	assert.Equal(t, "01JHANDLED", last.CausationID)
}
