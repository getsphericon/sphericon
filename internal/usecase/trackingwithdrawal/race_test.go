package trackingwithdrawal_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/order"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages/engagement"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/trackingwithdrawal"
)

// Order independence (ADR 0120, as ADR 0113 asks of Erasure): whatever the interleaving of a hit on
// a message, its stamp's projection, an Order's credit and the withdrawal of the message's
// tracking, once the withdrawal has committed nothing of the message is recorded and no later work
// records anything. The message is Emporium's Broadcast email its clicker Contact clicked
// (fixtures: the message, the click Event and the Contact share the id 7210).

const (
	message     = fixtures.OutboundEmporiumBroadcastClickID
	clicker     = fixtures.ContactClickerBroadcastID
	destination = "broadcast@emporium.test"
	personUA    = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15"
	personAddr  = "192.0.2.10" // outside every listed range
)

// writers are the four sides that meet the withdrawal: a person's open of the message, the
// projection of a click onto its stamp, an Order of its clicker (whose credit looks for the click)
// and the withdrawal itself, each in a transaction of its own.
func writers(t *testing.T, env *testhelper.TestEnv) map[string]testhelper.Writer {
	t.Helper()
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.EmporiumID)
	token, err := env.Tracker.Token(env.Tracked(t, message))
	require.NoError(t, err)
	hit := engagement.Hit{UserAgent: personUA, Address: personAddr, MayRecord: func() bool { return true }}
	placed := events.OrderEvent{
		Action:     orders.ActionCompleted,
		Properties: map[string]any{"order_id": "RACE-1", "total": 20.0, "currency": "USD"},
		ContactID:  clicker, OccurredAt: time.Now(), EventID: "completed-race-1",
	}
	ordersModule := testhelper.Module[*orders.Module](env)
	require.NoError(t, ordersModule.Prepare(ctx, s, placed))
	return map[string]testhelper.Writer{
		"open": {Name: "open", Run: func() error {
			return testhelper.Module[*engagement.Recorder](env).Open(ctx, token, hit)
		}},
		"projection": {Name: "projection", Run: func() error {
			_, err := testhelper.Module[*outboundmessages.Module](env).StampClicked(ctx, s, message, time.Now())
			return err
		}},
		"credit": {Name: "credit", Run: func() error {
			return env.Bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
				return ordersModule.Record(ctx, ts, placed)
			})
		}},
		"withdrawal": {Name: "withdrawal", Run: func() error {
			return testhelper.Module[*trackingwithdrawal.UseCase](env).Withdraw(ctx, s, trackingwithdrawal.Refusal{
				Channel: channels.Email, Destination: destination,
			})
		}},
	}
}

// requireNothingRecorded asserts that nothing of the message is left: no open or click Event, no
// stamp, no Order credited through it, and that its tracking is withdrawn. A race test reads the
// tables it raced on.
func requireNothingRecorded(t *testing.T, env *testhelper.TestEnv, errs map[string]error) {
	t.Helper()
	ctx := context.Background()
	for name, err := range errs {
		require.NoError(t, err, name)
	}
	s := env.DB.Scoped(fixtures.EmporiumID)
	engaged, err := s.Event().Query().Where(
		event.ContactID(clicker), event.ActionIn(systemevent.EmailOpened, systemevent.EmailClicked), events.NamesMessageIn([]int64{message}),
	).Count(ctx)
	require.NoError(t, err)
	msg, err := s.OutboundMessage().Query().Where(outboundmessage.ID(message)).Only(ctx)
	require.NoError(t, err)
	credited, err := s.Order().Query().Where(order.CreditedMessageID(message)).Count(ctx)
	require.NoError(t, err)
	placed, err := s.Order().Query().Where(order.OrderID("RACE-1")).Only(ctx)
	require.NoError(t, err)

	assert.Zero(t, engaged, "no open or click of the message is left")
	assert.Nil(t, msg.OpenedAt, "no open stamp")
	assert.Nil(t, msg.ClickedAt, "no click stamp")
	assert.NotNil(t, msg.TrackingWithdrawnAt, "the message is withdrawn")
	assert.Zero(t, credited, "no Order is credited through the message")
	assert.Nil(t, placed.BroadcastID, "the Order placed in the race has no credit")
	assert.Nil(t, placed.CreditedMessageID)
}

// The withdrawal first: it holds the message's row while it deletes what was recorded, and an open,
// a projection and a credit queued behind it find the message withdrawn and record nothing.
func TestWorkThatMeetsAWithdrawalInFlightWaitsAndRecordsNothing(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())
	w := writers(t, env)
	// The click Event held by a transaction of its own: the withdrawal takes the message's row,
	// then waits on the Event it deletes.
	release := env.HoldOpen(t, env.DB.Scoped(fixtures.EmporiumID), func(ts *ent.Scoped, _ events.Publisher) error {
		_, err := ts.Event().Query().Where(event.ID(fixtures.EventClickerBroadcastClickID)).ForUpdate().Only(context.Background())
		return err
	})

	errs := env.QueueBehind(t, release, w["withdrawal"], w["open"], w["projection"], w["credit"])

	requireNothingRecorded(t, env, errs)
}

// The other order: an open, a credit and a projection in flight when the withdrawal arrives. The
// withdrawal waits for each and then deletes what it recorded.
func TestAWithdrawalThatMeetsWorkInFlightWaitsAndDeletesWhatItRecorded(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())
	w := writers(t, env)
	// The clicker's row and the message's held by a transaction of its own: the open and the credit
	// take the message's row and then wait on the Contact their Event and Order name; the projection
	// waits on the message's row.
	release := env.HoldOpen(t, env.DB.Scoped(fixtures.EmporiumID), func(ts *ent.Scoped, _ events.Publisher) error {
		ctx := context.Background()
		if _, err := ts.Contact().Query().Where(contact.ID(clicker)).ForUpdate().Only(ctx); err != nil {
			return err
		}
		_, err := ts.OutboundMessage().Query().Where(outboundmessage.ID(message)).ForShare().Only(ctx)
		return err
	})

	errs := env.QueueBehind(t, release, w["open"], w["credit"], w["projection"], w["withdrawal"])

	requireNothingRecorded(t, env, errs)
}
