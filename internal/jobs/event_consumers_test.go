package jobs_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertest"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/heldorderevent"
	"github.com/getsphericon/sphericon/ent/order"
	"github.com/getsphericon/sphericon/ent/ordermovement"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
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
	keys := lo.Map(queued, func(e events.Envelope, _ int) string { return e.DedupKey })

	assert.Equal(t, []string{"email.bounced/hard@example.com", "email.complained/complained@example.com"}, keys)
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

	env.PerformEnqueuedJobs(t, jobs.SuppressEventArgs{})
	env.RetryJobs(t, jobs.SuppressEventArgs{})
	env.PerformEnqueuedJobs(t, jobs.SuppressEventArgs{})

	entries, err := env.DB.Suppression.Query().
		Where(entsuppression.WorkspaceID(fixtures.AcmeID), entsuppression.Destination("twice@example.com")).All(t.Context())
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, entsuppression.ReasonBounce, entries[0].Reason)
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
	bus := testhelper.BusOn(t, sqlDB)
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
	published := testhelper.QueuedEnvelopesOn(t, sqlDB, events.NameContactCreated)
	require.NotEmpty(t, published)
	last := published[len(published)-1]
	assert.Equal(t, "01JROOT", last.CorrelationID)
	assert.Equal(t, "01JHANDLED", last.CausationID)
}

// orderEvent is a collected order Event of Alice in Acme.
func orderEvent(action string, props map[string]any, backfill bool) *events.CollectedEvent {
	return &events.CollectedEvent{
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, SubjectID: "alice", Action: action,
		Properties: props, Backfill: backfill,
	}
}

// An order Event inserts one orders job, loaded history included; any other event inserts none.
func TestAnOrderEventQueuesOneOrdersJob(t *testing.T) {
	env := testhelper.Setup(t)
	publishEvent(t, env, orderEvent(orders.ActionCompleted, map[string]any{"order_id": "Q-1", "total": 20.0, "currency": "USD"}, false))
	publishEvent(t, env, orderEvent(orders.ActionRefunded, map[string]any{"order_id": "Q-0", "total": 5.0}, true))
	publishEvent(t, env, orderEvent("page_view", nil, false))
	publishEvent(t, env, notice(events.NameEmailBounced, events.BounceKindHard, "order-less@example.com"))

	orderJobs := env.EventJobs(t, jobs.OrderEventArgs{}.Kind())
	actions := collectedActions(t, orderJobs)

	assert.ElementsMatch(t, []string{orders.ActionCompleted, orders.ActionRefunded}, actions)
}

// collectedActions are the customer's actions of the collected events the envelopes carry.
func collectedActions(t *testing.T, envelopes []events.Envelope) []string {
	t.Helper()
	out := make([]string, 0, len(envelopes))
	for i := range envelopes {
		ev, err := events.Decode(envelopes[i])
		require.NoError(t, err)
		collected, ok := ev.(*events.CollectedEvent)
		require.True(t, ok, "%s is not a collected event", envelopes[i].Name)
		out = append(out, collected.Action)
	}
	return out
}

// fixedCurrency adopts nothing: the Workspace has its Reporting currency already.
type fixedCurrency struct{}

func (fixedCurrency) AdoptReportingCurrency(context.Context, *ent.Scoped, string) error { return nil }

// Two order Events of one Order worked at the same moment leave it as working them one after the
// other does, whichever runs first: the refund either finds the Order or is held until the Order
// Completed applies it. One transaction at a time reads a Workspace's order Events.
func TestOrderJobsWorkedConcurrentlyLeaveTheOrderAsWorkedInTurn(t *testing.T) {
	ctx := context.Background()
	sqlDB := testhelper.RealDB(t)
	sqlDB.SetMaxOpenConns(8)
	client := entclient.New(sqlDB)
	// test writes: the Workspace's Reporting currency, so no Order Completed adopts one.
	client.Workspace.UpdateOneID(fixtures.AcmeID).SetReportingCurrency("USD").ExecX(ctx)
	bus := testhelper.BusOn(t, sqlDB)
	inserter, err := jobs.NewInserter(sqlDB)
	require.NoError(t, err)
	for _, ev := range []*events.CollectedEvent{
		orderEvent(orders.ActionCompleted, map[string]any{"order_id": "C-1", "total": 100.0, "currency": "USD"}, false),
		orderEvent(orders.ActionRefunded, map[string]any{"order_id": "C-1", "total": 10.0, "refund_id": "R-1"}, false),
	} {
		require.NoError(t, bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error { return pub.Publish(ctx, ev) }))
	}
	listed, err := inserter.JobList(ctx, river.NewJobListParams().Kinds(jobs.OrderEventArgs{}.Kind()))
	require.NoError(t, err)
	require.Len(t, listed.Jobs, 2)
	w := rivertest.NewWorker(t, riverdatabasesql.New(sqlDB), jobs.BaseConfig(),
		river.Worker[jobs.OrderEventArgs](jobs.NewOrderEventWorker(client, orders.New(fixedCurrency{}))))

	// Each job is worked in a transaction of its own, opened before either starts.
	txs := make([]*sql.Tx, len(listed.Jobs))
	for i := range txs {
		txs[i], err = sqlDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		t.Cleanup(func() { cleanup.Report("roll the job's transaction back", txs[i].Rollback()) })
	}

	var wg sync.WaitGroup
	errs := make([]error, len(listed.Jobs))
	for i, job := range listed.Jobs {
		wg.Go(func() { _, errs[i] = w.WorkJob(ctx, t, txs[i], job) })
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
	o, err := client.Order.Query().Where(order.WorkspaceID(fixtures.AcmeID), order.OrderID("C-1")).Only(ctx)
	require.NoError(t, err)
	movements, err := client.OrderMovement.Query().Where(ordermovement.OrderID(o.ID)).All(ctx)
	require.NoError(t, err)
	held, err := client.HeldOrderEvent.Query().Where(heldorderevent.OrderID("C-1")).Count(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{10000, -1000}, lo.Map(movements, func(m *ent.OrderMovement, _ int) int64 { return m.Amount }))
	assert.Zero(t, held, "nothing waits for an Order that exists")
}
