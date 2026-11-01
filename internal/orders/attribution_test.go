package orders_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/order"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/orders"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Attribution (ADR 0064): an Order is credited, once, to the Broadcast or Automation whose email
// its Contact last clicked inside the Attribution window. Clicks are stored as the tracker and
// the persist subscriber leave them; the Orders Completed are handed to the module as the orders
// subscriber hands them over.

const day = 24 * time.Hour

func ago(d time.Duration) time.Time { return time.Now().Add(-d) }

// clicked stores one email.clicked Event of Alice at the given time.
func clicked(t *testing.T, env *testhelper.TestEnv, click events.EmailEngagement, at time.Time) {
	t.Helper()
	click.Action = events.NameEmailClicked
	click.WorkspaceID = fixtures.AcmeID
	click.ContactID = fixtures.ContactAliceID
	click.Email = fixtures.ContactAliceEmail
	click.URL = "https://shop.test/"
	require.NoError(t, env.Bus.WithinTx(t.Context(), func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(t.Context(), &click)
	}))
	sent := env.OutboxEnvelopes(t, events.NameEmailClicked)
	stored := sent[len(sent)-1]
	stored.OccurredAt = at
	require.NoError(t, events.Persist(t.Context(), env.DB, stored))
}

// completed is an Order Completed of 20.00 USD by the Contact at the given time.
func completed(id string, contactID int64, at time.Time) events.OrderEvent {
	return events.OrderEvent{
		Action:     orders.ActionCompleted,
		Properties: map[string]any{"order_id": id, "total": 20.0, "currency": "USD"},
		ContactID:  contactID, OccurredAt: at, EventID: "completed-" + id,
	}
}

// record hands the order Event to the module, as the orders subscriber does, and returns the
// Order it is about.
func record(t *testing.T, env *testhelper.TestEnv, ev events.OrderEvent) *ent.Order {
	t.Helper()
	s := env.DB.Scoped(fixtures.AcmeID)
	require.NoError(t, env.Orders.Prepare(t.Context(), s, ev))
	require.NoError(t, env.Orders.Record(t.Context(), s, ev))
	return orderOf(t, env, ev.Properties["order_id"].(string))
}

func orderOf(t *testing.T, env *testhelper.TestEnv, id string) *ent.Order {
	t.Helper()
	return env.DB.Scoped(fixtures.AcmeID).Order().Query().Where(order.OrderID(id)).OnlyX(t.Context())
}

// placeOrder is Alice's Order Completed now.
func placeOrder(t *testing.T, env *testhelper.TestEnv, id string) *ent.Order {
	t.Helper()
	return record(t, env, completed(id, fixtures.ContactAliceID, time.Now()))
}

func creditedTo(o *ent.Order) (broadcast, automation int64) {
	if o.BroadcastID != nil {
		broadcast = *o.BroadcastID
	}
	if o.AutomationID != nil {
		automation = *o.AutomationID
	}
	return broadcast, automation
}

func TestAnOrderIsCreditedToTheBroadcastTheContactClicked(t *testing.T) {
	env := testhelper.Setup(t)
	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID}, ago(day))

	o := placeOrder(t, env, "C-1")

	b, a := creditedTo(o)
	assert.EqualValues(t, fixtures.BroadcastSentUntrackedID, b)
	assert.Zero(t, a)
	require.NotNil(t, o.CreditedClickAt)
	assert.WithinDuration(t, ago(day), *o.CreditedClickAt, time.Minute)
}

func TestAnOrderIsCreditedToTheAutomationTheContactClicked(t *testing.T) {
	env := testhelper.Setup(t)
	clicked(t, env, events.EmailEngagement{AutomationID: fixtures.AutomationWelcomeSeriesID}, ago(day))

	b, a := creditedTo(placeOrder(t, env, "C-1"))

	assert.Zero(t, b)
	assert.EqualValues(t, fixtures.AutomationWelcomeSeriesID, a)
}

func TestTheLaterOfTwoClicksWinsWhateverCarriedIt(t *testing.T) {
	env := testhelper.Setup(t)
	clicked(t, env, events.EmailEngagement{AutomationID: fixtures.AutomationWelcomeSeriesID}, ago(3*day))
	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID}, ago(2*day))
	clicked(t, env, events.EmailEngagement{AutomationID: fixtures.AutomationWelcomeSeriesID}, ago(4*day))

	b, a := creditedTo(placeOrder(t, env, "C-1"))

	assert.EqualValues(t, fixtures.BroadcastSentUntrackedID, b)
	assert.Zero(t, a)
}

func TestOfTwoClicksAtTheSameTimeTheOneStoredLastWins(t *testing.T) {
	env := testhelper.Setup(t)
	at := ago(day)
	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID}, at)
	clicked(t, env, events.EmailEngagement{AutomationID: fixtures.AutomationWelcomeSeriesID}, at)

	b, a := creditedTo(placeOrder(t, env, "C-1"))

	assert.Zero(t, b)
	assert.EqualValues(t, fixtures.AutomationWelcomeSeriesID, a)
}

func TestAClickBySoftwareAndAClickOfNeitherCreditNothing(t *testing.T) {
	env := testhelper.Setup(t)
	clicked(t, env, events.EmailEngagement{AutomationID: fixtures.AutomationWelcomeSeriesID}, ago(3*day))
	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID, Machine: true}, ago(2*day))
	clicked(t, env, events.EmailEngagement{}, ago(day)) // a Transactional send carries neither

	b, a := creditedTo(placeOrder(t, env, "C-1"))

	assert.Zero(t, b, "the scanner's click is passed over")
	assert.EqualValues(t, fixtures.AutomationWelcomeSeriesID, a, "so is the click of no Broadcast or Automation")
}

func TestAClickBeforeTheWindowOrAfterTheOrderCreditsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID}, ago(6*day))
	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID}, time.Now().Add(time.Hour))

	o := placeOrder(t, env, "C-1")

	b, a := creditedTo(o)
	assert.Zero(t, b, "a click six days old is outside the 5 days; one after the order is no cause of it")
	assert.Zero(t, a)
	assert.Nil(t, o.CreditedClickAt)
}

func TestTheWindowIsTheWorkspacesAndAChangeLaterMovesNoOrder(t *testing.T) {
	env := testhelper.Setup(t)
	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID}, ago(4*day))
	first := placeOrder(t, env, "C-1")

	require.NoError(t, env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetAttributionWindowDays(1).Exec(t.Context()))
	second := placeOrder(t, env, "C-2")

	b1, _ := creditedTo(orderOf(t, env, first.OrderID))
	b2, _ := creditedTo(second)
	assert.EqualValues(t, fixtures.BroadcastSentUntrackedID, b1, "credited under the 5 days it arrived in")
	assert.Zero(t, b2, "the 4 day old click is outside the 1 day that holds now")
}

func TestAClickStoredAfterTheOrderCreditsNothingThoughDatedBeforeIt(t *testing.T) {
	env := testhelper.Setup(t)
	ev := completed("C-1", fixtures.ContactAliceID, time.Now())
	record(t, env, ev)

	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID}, ago(time.Hour))
	kept := record(t, env, ev) // the order again: the credit is not looked for a second time

	b, a := creditedTo(kept)
	assert.Zero(t, b)
	assert.Zero(t, a)
}

func TestAnOrderOfNoKnownContactIsUnattributed(t *testing.T) {
	env := testhelper.Setup(t)
	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID}, ago(day))

	guest := record(t, env, completed("G-1", 0, time.Now()))

	b, a := creditedTo(guest)
	assert.Zero(t, b)
	assert.Zero(t, a)
	assert.Nil(t, guest.ContactID)
}

// An order Event that arrives before its Order Completed is held and applied once the Order
// exists: its Movement goes to the Broadcast the Order is credited to.
func TestAnEarlyRefundMovesTheRevenueOfTheMessageTheOrderIsCreditedTo(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	refund := events.OrderEvent{
		Action:     orders.ActionRefunded,
		Properties: map[string]any{"order_id": "C-1", "total": 5.0, "currency": "USD"},
		ContactID:  fixtures.ContactAliceID, OccurredAt: ago(time.Hour), EventID: "refunded-C-1",
	}
	require.NoError(t, env.Orders.Record(t.Context(), s, refund))
	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID}, ago(day))

	placeOrder(t, env, "C-1")

	got, err := orders.CreditedToBroadcasts(t.Context(), s, fixtures.BroadcastSentUntrackedID)
	require.NoError(t, err)
	assert.Equal(t, orders.Credited{Orders: 1, Amount: 1500}, got.ByID[fixtures.BroadcastSentUntrackedID])
}

// Retention counts from when an Event is stored (ADR 0019, ADR 0064): once the winning click
// has been pruned a Backfilled Order has nothing to be credited to.
func TestABackfilledOrderWhoseClickWasPrunedIsUnattributed(t *testing.T) {
	const retention = 30 * day
	env := testhelper.Setup(t)
	// Stored a day past the window, so the prune takes it.
	// test writes: an Event's storage time is the clock's, so only a direct write can give a click the age retention counts from.
	env.DB.Event.Create().SetWorkspaceID(fixtures.AcmeID).
		SetSubjectID("alice").SetAction(events.NameEmailClicked).
		SetContactID(fixtures.ContactAliceID).SetEmail(fixtures.ContactAliceEmail).
		SetProperties(map[string]any{"broadcastId": fixtures.BroadcastSentUntrackedID}).
		SetBroadcastID(fixtures.BroadcastSentUntrackedID).
		SetOccurredAt(ago(retention + day)).SetCreatedAt(ago(retention + day)).ExecX(t.Context())
	first := record(t, env, completed("R-1", fixtures.ContactAliceID, ago(retention)))
	b, _ := creditedTo(first)
	require.EqualValues(t, fixtures.BroadcastSentUntrackedID, b, "while the click is kept it credits a Backfilled Order")

	pruned, err := events.PruneEvents(t.Context(), env.DB, retention, 100)
	require.NoError(t, err)
	require.NotZero(t, pruned)
	second := record(t, env, completed("R-2", fixtures.ContactAliceID, ago(retention)))

	b, a := creditedTo(second)
	assert.Zero(t, b)
	assert.Zero(t, a)
	assert.Nil(t, second.CreditedClickAt)
}
