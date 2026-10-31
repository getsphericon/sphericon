package events_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// TestDomainEventsRouterDelivery proves the full transport end-to-end against the
// real DB (not txdb): a producer publishes through the transactional outbox, and
// the watermill-sql router reads that same table and delivers the decoded
// envelope to a subscriber. It uses a capture handler instead of persist so it
// writes no ent rows (no FK setup, no fixture-count interference). The database is
// the test's own, so what it commits is dropped with it.
func TestDomainEventsRouterDelivery(t *testing.T) {
	sqlDB := testhelper.RealDB(t)
	ctx := context.Background()

	router, err := events.NewRouter(sqlDB)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(router, "router") })
	sub, err := events.NewSubscriber(sqlDB, "capture", testhelper.PrivateOutbox())
	require.NoError(t, err)

	got := make(chan events.Envelope, 1)
	router.AddConsumerHandler("capture", events.TopicDomainEvents, sub,
		func(msg *message.Message) error {
			var env events.Envelope
			if err := json.Unmarshal(msg.Payload, &env); err != nil {
				return err
			}
			select {
			case got <- env:
			default:
			}
			return nil
		})

	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go func() { cleanup.Report("run the router", router.Run(runCtx)) }()
	<-router.Running()

	bus := events.New(sqlDB)
	require.NoError(t, bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(ctx, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: 7, Email: "e2e@example.com"})
	}))

	env := testhelper.Receive(t, got, "domain event was not delivered to the subscriber")
	ev, err := events.Decode(env)
	require.NoError(t, err)
	assert.Equal(t, events.NameContactCreated, env.Name)
	assert.EqualValues(t, fixtures.AcmeID, env.WorkspaceID)
	assert.Equal(t, "e2e@example.com", ev.Project().Subject)
}

// A transaction left open in another database of the shared test server must not hold
// back delivery in this one: the default watermill horizon (pg_snapshot_xmin) is
// cluster-wide, which made the router tests wait on whatever else was running.
func TestDeliveryIgnoresTransactionsOfOtherDatabases(t *testing.T) {
	ctx := context.Background()
	other := db.NewEntClient(testhelper.RealDB(t))
	tx, err := other.Tx(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	// test writes: transaction commit and rollback through the bus is the subject
	_, err = tx.Client().Scoped(fixtures.AcmeID).Tag().Create().SetName("held-open").Save(ctx)
	require.NoError(t, err)

	sqlDB := testhelper.RealDB(t)
	router, err := events.NewRouter(sqlDB)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(router, "router") })
	sub, err := events.NewSubscriber(sqlDB, "capture", testhelper.PrivateOutbox())
	require.NoError(t, err)
	got := make(chan events.Envelope, 1)
	router.AddConsumerHandler("capture", events.TopicDomainEvents, sub, func(msg *message.Message) error {
		var env events.Envelope
		if err := json.Unmarshal(msg.Payload, &env); err != nil {
			return err
		}
		got <- env
		return nil
	})
	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go func() { cleanup.Report("run the router", router.Run(runCtx)) }()
	<-router.Running()

	require.NoError(t, events.New(sqlDB).WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(ctx, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: 7, Email: "other-db@example.com"})
	}))

	env := testhelper.Receive(t, got, "a transaction of another database held back delivery")
	assert.Equal(t, events.NameContactCreated, env.Name)
}
