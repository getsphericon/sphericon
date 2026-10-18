package events_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// RegisterSubscribers wires the four consumers onto one router; a published hard
// bounce reaches every one of them. It runs on a database of its own (the router
// reads the committed outbox), dropped with the test.
func TestRegisterSubscribersFansEveryEventOutToAllConsumers(t *testing.T) {
	sqlDB := testhelper.RealDB(t)
	client := db.NewEntClient(sqlDB)

	ctx := context.Background()
	const dest = "registered-bounce@example.com"

	router, err := events.NewRouter()
	require.NoError(t, err)
	t.Cleanup(func() { _ = router.Close() })
	enroller := &EnrollerMock{OnEventFunc: func(context.Context, int64, int64, string) error { return nil }}
	dispatcher := &WebhookDispatcherMock{DispatchFunc: func(context.Context, *ent.Scoped, string, string, []byte) error { return nil }}
	require.NoError(t, events.RegisterSubscribers(router, sqlDB, client, enroller, dispatcher))

	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go func() { _ = router.Run(runCtx) }()
	<-router.Running()

	require.NoError(t, events.New(sqlDB).WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(ctx, &events.EmailDeliveryFailure{
			Action: events.NameEmailBounced, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID,
			Email: dest, BounceKind: events.BounceKindPermanent, DedupID: "registered-bounce-1",
		})
	}))

	require.Eventually(t, func() bool {
		persisted, _ := client.Event.Query().Where(event.SubjectID(dest)).Exist(ctx)
		suppressed, _ := client.Suppression.Query().Where(suppression.Destination(dest)).Exist(ctx)
		return persisted && suppressed && len(enroller.OnEventCalls()) >= 1 && len(dispatcher.DispatchCalls()) >= 1
	}, 15*time.Second, 50*time.Millisecond, "persist, suppression, automations and webhooks must all see the event")
	// Delivery is at-least-once: a serialization failure while acking redelivers the
	// message, so a consumer may see it more than once, never a different event.
	for _, c := range dispatcher.DispatchCalls() {
		assert.Equal(t, events.NameEmailBounced, c.EventName)
	}
}
