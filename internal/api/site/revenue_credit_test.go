package site_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/order"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Credit (ADR 0064): an Order is credited, once, to the Broadcast or Automation whose email its
// Contact last clicked inside the Attribution window.

// clicked stores one email.clicked Event of Alice at the given time, as the tracker and the
// persist subscriber would have left it.
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

func ago(d time.Duration) time.Time { return time.Now().Add(-d) }

const day = 24 * time.Hour

// placeOrder sends Alice's Order Completed, delivers it and returns the stored Order.
func placeOrder(t *testing.T, env *testhelper.TestEnv, id string) *ent.Order {
	t.Helper()
	newShop(t, env).completed(id, "20.00", "USD")
	env.DeliverOrders(t)
	return env.DB.Scoped(fixtures.AcmeID).Order().Query().Where(order.OrderID(id)).OnlyX(t.Context())
}

func creditedTo(t *testing.T, o *ent.Order) (broadcast, automation int64) {
	t.Helper()
	if o.BroadcastID != nil {
		broadcast = *o.BroadcastID
	}
	if o.AutomationID != nil {
		automation = *o.AutomationID
	}
	return broadcast, automation
}

func setupCredit(t *testing.T) *testhelper.TestEnv {
	t.Helper()
	env := testhelper.Setup(t)
	chooseCurrency(t, env.SiteActor(t, fixtures.OwnerJohnEmail), "USD")
	return env
}

func TestAnOrderIsCreditedToTheBroadcastTheContactClicked(t *testing.T) {
	env := setupCredit(t)
	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID}, ago(day))

	o := placeOrder(t, env, "C-1")

	b, a := creditedTo(t, o)
	assert.EqualValues(t, fixtures.BroadcastSentUntrackedID, b)
	assert.Zero(t, a)
	require.NotNil(t, o.CreditedClickAt)
	assert.WithinDuration(t, ago(day), *o.CreditedClickAt, time.Minute)
	got := revenue(t, env.SiteActor(t, fixtures.MemberMaryEmail), siteapi.OptSiteAnalyticsRevenueRange{})
	assert.EqualValues(t, 2000, got.Total)
	assert.EqualValues(t, 2000, got.Credited)
	assert.EqualValues(t, 1, got.CreditedOrders)
}

func TestAnOrderIsCreditedToTheAutomationTheContactClicked(t *testing.T) {
	env := setupCredit(t)
	clicked(t, env, events.EmailEngagement{AutomationID: fixtures.AutomationWelcomeSeriesID}, ago(day))

	b, a := creditedTo(t, placeOrder(t, env, "C-1"))

	assert.Zero(t, b)
	assert.EqualValues(t, fixtures.AutomationWelcomeSeriesID, a)
}

func TestTheLaterOfTwoClicksWinsWhateverCarriedIt(t *testing.T) {
	env := setupCredit(t)
	clicked(t, env, events.EmailEngagement{AutomationID: fixtures.AutomationWelcomeSeriesID}, ago(3*day))
	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID}, ago(2*day))
	clicked(t, env, events.EmailEngagement{AutomationID: fixtures.AutomationWelcomeSeriesID}, ago(4*day))

	b, a := creditedTo(t, placeOrder(t, env, "C-1"))

	assert.EqualValues(t, fixtures.BroadcastSentUntrackedID, b)
	assert.Zero(t, a)
}

func TestOfTwoClicksAtTheSameTimeTheOneStoredLastWins(t *testing.T) {
	env := setupCredit(t)
	at := ago(day)
	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID}, at)
	clicked(t, env, events.EmailEngagement{AutomationID: fixtures.AutomationWelcomeSeriesID}, at)

	b, a := creditedTo(t, placeOrder(t, env, "C-1"))

	assert.Zero(t, b)
	assert.EqualValues(t, fixtures.AutomationWelcomeSeriesID, a)
}

func TestAClickBySoftwareAndAClickOfNeitherCreditNothing(t *testing.T) {
	env := setupCredit(t)
	clicked(t, env, events.EmailEngagement{AutomationID: fixtures.AutomationWelcomeSeriesID}, ago(3*day))
	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID, Machine: true}, ago(2*day))
	clicked(t, env, events.EmailEngagement{}, ago(day)) // a Transactional send carries neither

	b, a := creditedTo(t, placeOrder(t, env, "C-1"))

	assert.Zero(t, b, "the scanner's click is passed over")
	assert.EqualValues(t, fixtures.AutomationWelcomeSeriesID, a, "so is the click of no Broadcast or Automation")
}

func TestAnOrderWithNoClickInsideTheWindowIsUnattributedAndStillCounts(t *testing.T) {
	env := setupCredit(t)
	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID}, ago(6*day))
	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID}, time.Now().Add(time.Hour))

	o := placeOrder(t, env, "C-1")

	b, a := creditedTo(t, o)
	assert.Zero(t, b)
	assert.Zero(t, a)
	assert.Nil(t, o.CreditedClickAt)
	got := revenue(t, env.SiteActor(t, fixtures.MemberMaryEmail), siteapi.OptSiteAnalyticsRevenueRange{})
	assert.EqualValues(t, 2000, got.Total, "a click six days old is outside the 5 days; one after the order is no cause of it")
	assert.Zero(t, got.Credited)
	assert.Zero(t, got.CreditedOrders)
}

func TestTheWindowIsTheWorkspacesAndAChangeLaterMovesNoOrder(t *testing.T) {
	env := setupCredit(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID}, ago(4*day))
	first := placeOrder(t, env, "C-1")

	res, err := owner.SiteWorkspacesUpdateRevenueSettings(t.Context(),
		&siteapi.SiteUpdateRevenueSettingsInput{AttributionWindowDays: siteapi.NewOptAttributionWindowDays(1)},
		siteapi.SiteWorkspacesUpdateRevenueSettingsParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteRevenueSettingsResource{}, res)
	second := placeOrder(t, env, "C-2")

	b1, _ := creditedTo(t, first)
	b2, _ := creditedTo(t, second)
	assert.EqualValues(t, fixtures.BroadcastSentUntrackedID, b1, "credited under the 5 days it arrived in")
	assert.Zero(t, b2, "the 4 day old click is outside the 1 day that holds now")
	got := revenue(t, owner, siteapi.OptSiteAnalyticsRevenueRange{})
	assert.EqualValues(t, 2000, got.Credited)
}

func TestAClickStoredAfterTheOrderCreditsNothingThoughDatedBeforeIt(t *testing.T) {
	env := setupCredit(t)
	o := placeOrder(t, env, "C-1")

	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID}, ago(time.Hour))
	env.DeliverOrders(t) // the order again: the credit is not looked for a second time

	kept := env.DB.Scoped(fixtures.AcmeID).Order().Query().Where(order.ID(o.ID)).OnlyX(t.Context())
	b, a := creditedTo(t, kept)
	assert.Zero(t, b)
	assert.Zero(t, a)
}

func TestAnOrderOfAnUnknownContactIsUnattributed(t *testing.T) {
	env := setupCredit(t)
	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID}, ago(day))
	in := orderInput("guest-checkout-1", "Order Completed", map[string]string{"order_id": `"G-1"`, "total": `7.25`, "currency": `"USD"`})
	in.Email = externalapi.OptNilEmailAddress{}
	newShop(t, env).send(in)
	env.DeliverOrders(t)

	guest := env.DB.Scoped(fixtures.AcmeID).Order().Query().Where(order.OrderID("G-1")).OnlyX(t.Context())

	b, a := creditedTo(t, guest)
	assert.Zero(t, b)
	assert.Zero(t, a)
	got := revenue(t, env.SiteActor(t, fixtures.MemberMaryEmail), siteapi.OptSiteAnalyticsRevenueRange{})
	assert.EqualValues(t, 725, got.Total)
	assert.Zero(t, got.Credited)
}

// Retention counts from when an Event is stored (ADR 0019, ADR 0064): once the winning click
// has been pruned a Backfilled Order has nothing to be credited to, yet its revenue stays. The
// window is the instance's (400 days by default); a 30 day one keeps the Orders inside the 90
// days the overview reads.
func TestABackfilledOrderWhoseClickWasPrunedIsUnattributedAndStillCounts(t *testing.T) {
	const retention = 30 * day
	env := setupCredit(t)
	s := newShop(t, env)
	// Stored a day past the window, so the prune takes it.
	// test writes: an Event's storage time is the clock's, so only a direct write can give a click the age retention counts from.
	env.DB.Event.Create().SetWorkspaceID(fixtures.AcmeID).
		SetSubjectID("alice").SetAction(events.NameEmailClicked).
		SetContactID(fixtures.ContactAliceID).SetEmail(fixtures.ContactAliceEmail).
		SetProperties(map[string]any{"broadcastId": fixtures.BroadcastSentUntrackedID}).
		SetBroadcastID(fixtures.BroadcastSentUntrackedID).
		SetOccurredAt(ago(retention + day)).SetCreatedAt(ago(retention + day)).ExecX(t.Context())
	s.at(ago(retention), "Order Completed", orderProps("R-1", "20.00"))
	env.DeliverOrders(t)
	first := env.DB.Scoped(fixtures.AcmeID).Order().Query().Where(order.OrderID("R-1")).OnlyX(t.Context())
	b, _ := creditedTo(t, first)
	require.EqualValues(t, fixtures.BroadcastSentUntrackedID, b, "while the click is kept it credits a Backfilled Order")

	pruned, err := events.PruneEvents(t.Context(), env.DB, retention, 100)
	require.NoError(t, err)
	require.NotZero(t, pruned)
	s.at(ago(retention), "Order Completed", orderProps("R-2", "30.00"))
	env.DeliverOrders(t)

	second := env.DB.Scoped(fixtures.AcmeID).Order().Query().Where(order.OrderID("R-2")).OnlyX(t.Context())
	b, a := creditedTo(t, second)
	assert.Zero(t, b)
	assert.Zero(t, a)
	assert.Nil(t, second.CreditedClickAt)
	got := revenue(t, env.SiteActor(t, fixtures.MemberMaryEmail), siteapi.NewOptSiteAnalyticsRevenueRange(siteapi.SiteAnalyticsRevenueRange90d))
	assert.EqualValues(t, 5000, got.Total, "the unattributed Order still counts in total revenue")
	assert.EqualValues(t, 2000, got.Credited)
}
