package orders_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/order"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Attribution (ADR 0064): an Order is credited, once, to the Broadcast or Automation whose email
// its Contact last clicked inside the Attribution window. The clicks are fixture Events of
// Emporium (fixtures/events.yml), one Contact per click history; the Orders Completed are handed
// to the module as the orders consumer hands them over.

const day = 24 * time.Hour

func ago(d time.Duration) time.Time { return time.Now().Add(-d) }

// completed is an Order Completed of 20.00 USD by the Contact at the given time.
func completed(id string, contactID int64, at time.Time) events.OrderEvent {
	return events.OrderEvent{
		Action:     orders.ActionCompleted,
		Properties: map[string]any{"order_id": id, "total": 20.0, "currency": "USD"},
		ContactID:  contactID, OccurredAt: at, EventID: "completed-" + id,
	}
}

// record hands the order Event to the module in Emporium, as the orders subscriber does, and
// returns the Order it is about.
func record(t *testing.T, env *testhelper.TestEnv, ev events.OrderEvent) *ent.Order {
	t.Helper()
	s := env.DB.Scoped(fixtures.EmporiumID)
	require.NoError(t, env.Orders.Prepare(t.Context(), s, ev))
	require.NoError(t, env.Orders.Record(t.Context(), s, ev))
	return orderOf(t, env, ev.Properties["order_id"].(string))
}

func orderOf(t *testing.T, env *testhelper.TestEnv, id string) *ent.Order {
	t.Helper()
	return env.DB.Scoped(fixtures.EmporiumID).Order().Query().Where(order.OrderID(id)).OnlyX(t.Context())
}

// placeOrder is the Contact's Order Completed now.
func placeOrder(t *testing.T, env *testhelper.TestEnv, contactID int64, id string) *ent.Order {
	t.Helper()
	return record(t, env, completed(id, contactID, time.Now()))
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
	click := env.DB.Event.GetX(t.Context(), fixtures.EventClickerBroadcastClickID)

	o := placeOrder(t, env, fixtures.ContactClickerBroadcastID, "C-1")

	b, a := creditedTo(o)
	assert.EqualValues(t, fixtures.BroadcastEmporiumID, b)
	assert.Zero(t, a)
	require.NotNil(t, o.CreditedClickAt)
	assert.WithinDuration(t, click.OccurredAt, *o.CreditedClickAt, 0, "the time of the click")
}

func TestAnOrderIsCreditedToTheAutomationTheContactClicked(t *testing.T) {
	env := testhelper.Setup(t)

	b, a := creditedTo(placeOrder(t, env, fixtures.ContactClickerAutomationID, "C-1"))

	assert.Zero(t, b)
	assert.EqualValues(t, fixtures.AutomationEmporiumClicksID, a)
}

// Clicks: the Automation 3 and 4 days ago, the Broadcast 2 days ago.
func TestTheLaterOfTwoClicksWinsWhateverCarriedIt(t *testing.T) {
	env := testhelper.Setup(t)

	b, a := creditedTo(placeOrder(t, env, fixtures.ContactClickerLaterWinsID, "C-1"))

	assert.EqualValues(t, fixtures.BroadcastEmporiumID, b)
	assert.Zero(t, a)
}

// Clicks: the Broadcast, then the Automation, stored in that order at one time a day ago.
func TestOfTwoClicksAtTheSameTimeTheOneStoredLastWins(t *testing.T) {
	env := testhelper.Setup(t)

	b, a := creditedTo(placeOrder(t, env, fixtures.ContactClickerSameTimeID, "C-1"))

	assert.Zero(t, b)
	assert.EqualValues(t, fixtures.AutomationEmporiumClicksID, a)
}

// Clicks: the Automation 3 days ago, the Broadcast by software 2 days ago, a Transactional send
// (neither) a day ago.
func TestAClickBySoftwareAndAClickOfNeitherCreditNothing(t *testing.T) {
	env := testhelper.Setup(t)

	b, a := creditedTo(placeOrder(t, env, fixtures.ContactClickerSoftwareID, "C-1"))

	assert.Zero(t, b, "the scanner's click is passed over")
	assert.EqualValues(t, fixtures.AutomationEmporiumClicksID, a, "so is the click of no Broadcast or Automation")
}

// Clicks: the Broadcast 6 days ago and a day ahead.
func TestAClickBeforeTheWindowOrAfterTheOrderCreditsNothing(t *testing.T) {
	env := testhelper.Setup(t)

	o := placeOrder(t, env, fixtures.ContactClickerOutsideWindowID, "C-1")

	b, a := creditedTo(o)
	assert.Zero(t, b, "a click six days old is outside the 5 days; one after the order is no cause of it")
	assert.Zero(t, a)
	assert.Nil(t, o.CreditedClickAt)
}

// Clicks: the Broadcast 4 days ago.
func TestTheWindowIsTheWorkspacesAndAChangeLaterMovesNoOrder(t *testing.T) {
	env := testhelper.Setup(t)
	first := placeOrder(t, env, fixtures.ContactClickerWindowChangeID, "C-1")

	require.NoError(t, env.DB.Workspace.UpdateOneID(fixtures.EmporiumID).SetAttributionWindowDays(1).Exec(t.Context()))
	second := placeOrder(t, env, fixtures.ContactClickerWindowChangeID, "C-2")

	b1, _ := creditedTo(orderOf(t, env, first.OrderID))
	b2, _ := creditedTo(second)
	assert.EqualValues(t, fixtures.BroadcastEmporiumID, b1, "credited under the 5 days it arrived in")
	assert.Zero(t, b2, "the 4 day old click is outside the 1 day that holds now")
}

// The click is stored by the tracker's publish, after the Order arrived: the Order is dated an
// hour ahead, so the click, stored now, is dated before it.
func TestAClickStoredAfterTheOrderCreditsNothingThoughDatedBeforeIt(t *testing.T) {
	env := testhelper.Setup(t)
	ev := completed("C-1", fixtures.ContactClickerNoneID, time.Now().Add(time.Hour))
	record(t, env, ev)

	require.NoError(t, env.Bus.WithinTx(t.Context(), func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(t.Context(), &events.EmailEngagement{
			Action: events.NameEmailClicked, WorkspaceID: fixtures.EmporiumID, BroadcastID: fixtures.BroadcastEmporiumID,
			ContactID: fixtures.ContactClickerNoneID, Email: fixtures.ContactClickerNoneEmail, URL: "https://shop.test/",
		})
	}))
	kept := record(t, env, ev) // the order again: the credit is not looked for a second time
	fresh := record(t, env, completed("C-2", fixtures.ContactClickerNoneID, ev.OccurredAt))

	b, a := creditedTo(kept)
	assert.Zero(t, b)
	assert.Zero(t, a)
	b, _ = creditedTo(fresh)
	assert.EqualValues(t, fixtures.BroadcastEmporiumID, b, "a new Order at the same time is credited by the click")
}

func TestAnOrderOfNoKnownContactIsUnattributed(t *testing.T) {
	env := testhelper.Setup(t)

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
	s := env.DB.Scoped(fixtures.EmporiumID)
	refund := events.OrderEvent{
		Action:     orders.ActionRefunded,
		Properties: map[string]any{"order_id": "C-1", "total": 5.0, "currency": "USD"},
		ContactID:  fixtures.ContactClickerBroadcastID, OccurredAt: ago(time.Hour), EventID: "refunded-C-1",
	}
	require.NoError(t, env.Orders.Record(t.Context(), s, refund))

	placeOrder(t, env, fixtures.ContactClickerBroadcastID, "C-1")

	got, err := orders.CreditedToBroadcasts(t.Context(), s, fixtures.BroadcastEmporiumID)
	require.NoError(t, err)
	assert.Equal(t, orders.Credited{Orders: 1, Amount: 1500}, got.ByID[fixtures.BroadcastEmporiumID])
}

// Retention counts from when an Event is stored (ADR 0019, ADR 0064): once the winning click
// has been pruned a Backfilled Order has nothing to be credited to. The click was stored 31 days
// ago, a day past the window, so the prune takes it.
func TestABackfilledOrderWhoseClickWasPrunedIsUnattributed(t *testing.T) {
	const retention = 30 * day
	env := testhelper.Setup(t)
	first := record(t, env, completed("R-1", fixtures.ContactClickerPrunedID, ago(retention)))
	b, _ := creditedTo(first)
	require.EqualValues(t, fixtures.BroadcastEmporiumID, b, "while the click is kept it credits a Backfilled Order")

	pruned, err := events.PruneEvents(t.Context(), env.DB, retention, 100, time.Now())
	require.NoError(t, err)
	require.NotZero(t, pruned)
	second := record(t, env, completed("R-2", fixtures.ContactClickerPrunedID, ago(retention)))

	b, a := creditedTo(second)
	assert.Zero(t, b)
	assert.Zero(t, a)
	assert.Nil(t, second.CreditedClickAt)
}

// Each case is a Contact whose last two clicks, 3 days and a day ago, came by two Channels.
func TestAnOrderIsCreditedToTheSourceOfTheLastClickWhateverTheChannel(t *testing.T) {
	for _, tc := range []struct {
		name       string
		contactID  int64
		broadcast  int64
		automation int64
		why        string
	}{
		{"a Telegram click after an email click", fixtures.ContactClickerTelegramAfterEmailID, 0, fixtures.AutomationEmporiumClicksID, "the Telegram message's source"},
		{"an email click after a Telegram click", fixtures.ContactClickerEmailAfterTelegramID, fixtures.BroadcastEmporiumID, 0, "the email's source"},
		{"a Telegram click by software credits nothing", fixtures.ContactClickerTelegramMachineID, fixtures.BroadcastEmporiumID, 0, "the preview fetcher's click is passed over"},
		{"an Order after an SMS click is credited to the SMS", fixtures.ContactClickerSmsAfterEmailID, 0, fixtures.AutomationEmporiumClicksID, "the SMS's source"},
		{"a later email click takes the credit from an SMS click", fixtures.ContactClickerEmailAfterSmsID, fixtures.BroadcastEmporiumID, 0, "the email's source"},
		{"an SMS link preview fetch credits nothing", fixtures.ContactClickerSmsMachineID, fixtures.BroadcastEmporiumID, 0, "the preview fetch is passed over"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)

			b, a := creditedTo(placeOrder(t, env, tc.contactID, "C-1"))

			assert.Equal(t, tc.broadcast, b, tc.why)
			assert.Equal(t, tc.automation, a, tc.why)
		})
	}
}
