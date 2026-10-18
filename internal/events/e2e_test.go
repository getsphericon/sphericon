package events_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDomainEventsRouterDelivery proves the full transport end-to-end against the
// real DB (not txdb): a producer publishes through the transactional outbox, and
// the watermill-sql router reads that same table and delivers the decoded
// envelope to a subscriber. It uses a capture handler instead of persist so it
// writes no ent rows (no FK setup, no fixture-count interference). The database is
// the test's own, so what it commits is dropped with it.
func TestDomainEventsRouterDelivery(t *testing.T) {
	db := testhelper.RealDB(t)
	ctx := context.Background()

	router, err := events.NewRouter()
	require.NoError(t, err)
	t.Cleanup(func() { _ = router.Close() })
	sub, err := events.NewSubscriber(db, "capture")
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
	go func() { _ = router.Run(runCtx) }()
	<-router.Running()

	bus := events.New(db)
	require.NoError(t, bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(ctx, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: 7, Email: "e2e@example.com"})
	}))

	select {
	case env := <-got:
		assert.Equal(t, events.NameContactCreated, env.Name)
		assert.EqualValues(t, fixtures.AcmeID, env.WorkspaceID)
		ev, err := events.Decode(env)
		require.NoError(t, err)
		assert.Equal(t, "e2e@example.com", ev.Project().Subject)
	case <-time.After(10 * time.Second):
		t.Fatal("domain event was not delivered to the subscriber within 10s")
	}
}
