package site_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Orders and Revenue of a Broadcast and of an Automation (ADR 0064): read from the Movements of
// the Orders credited to it, in the Reporting currency, never stored. Which Order is credited to
// which is the orders module's, tested there; these tests read what the reports sum.

// clicked stores one email.clicked Event of Alice at the given time, as the tracker and the
// persist subscriber would have left it: the click an Order of hers after it is credited by.
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

func setupCredit(t *testing.T) *testhelper.TestEnv {
	t.Helper()
	env := testhelper.Setup(t)
	chooseCurrency(t, env.SiteActor(t, fixtures.OwnerJohnEmail), "USD")
	return env
}

func broadcastRevenue(t *testing.T, c *siteapi.Client, id int64) siteapi.SiteMessageRevenue {
	t.Helper()
	res, err := c.SiteBroadcastsGet(t.Context(), siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(id)})
	require.NoError(t, err)
	b, ok := res.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", res)
	return b.Stats.Revenue
}

func broadcastListRevenue(t *testing.T, c *siteapi.Client, id int64) siteapi.SiteMessageRevenue {
	t.Helper()
	res, err := c.SiteBroadcastsList(t.Context(), siteapi.SiteBroadcastsListParams{WorkspaceId: fixtures.AcmePublicID, PageSize: siteapi.NewOptInt32(100)})
	require.NoError(t, err)
	list, ok := res.(*siteapi.SiteBroadcastsListOK)
	require.Truef(t, ok, "got %T", res)
	for i := range list.Items {
		if int64(list.Items[i].ID) == id {
			return list.Items[i].Stats.Revenue
		}
	}
	require.FailNow(t, "broadcast not listed")
	return siteapi.SiteMessageRevenue{}
}

func automationRevenue(t *testing.T, c *siteapi.Client, id int64) (report, listed, got siteapi.SiteMessageRevenue) {
	t.Helper()
	rep, err := c.SiteAutomationReportsGet(t.Context(), siteapi.SiteAutomationReportsGetParams{WorkspaceId: fixtures.AcmePublicID, AutomationId: idStr(id)})
	require.NoError(t, err)
	r, ok := rep.(*siteapi.SiteAutomationReport)
	require.Truef(t, ok, "got %T", rep)
	lst, err := c.SiteAutomationsList(t.Context(), siteapi.SiteAutomationsListParams{WorkspaceId: fixtures.AcmePublicID, PageSize: siteapi.NewOptInt32(100)})
	require.NoError(t, err)
	l, ok := lst.(*siteapi.SiteAutomationsListOK)
	require.Truef(t, ok, "got %T", lst)
	for i := range l.Items {
		if int64(l.Items[i].ID) == id {
			listed = l.Items[i].Revenue.Value
		}
	}
	one, err := c.SiteAutomationsGet(t.Context(), siteapi.SiteAutomationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(id)})
	require.NoError(t, err)
	a, ok := one.(*siteapi.SiteAutomationResource)
	require.Truef(t, ok, "got %T", one)
	return r.Revenue, listed, a.Revenue.Value
}

func TestABroadcastShowsItsOrdersAndNetRevenueAndARefundLowersItOnItsDay(t *testing.T) {
	env := setupCredit(t)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)
	s := newShop(t, env)
	id := int64(fixtures.BroadcastSentUntrackedID)
	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID}, noonDaysAgo(4))

	s.at(noonDaysAgo(3), "Order Completed", orderProps("B-1", "20.00"))
	s.at(noonDaysAgo(3), "Order Completed", orderProps("B-2", "30.00", "currency", `"EUR"`)) // another currency: counted in no sum
	env.DeliverOrders(t)
	before := broadcastRevenue(t, c, id)
	assert.Equal(t, siteapi.NewNilCurrencyCode("USD"), before.ReportingCurrency)
	assert.EqualValues(t, 1, before.Orders)
	assert.EqualValues(t, 2000, before.Amount)
	_, days := byDay(t, env)
	assert.EqualValues(t, 2000, days[reportDay(3)])

	s.at(noonDaysAgo(1), "Order Refunded", orderProps("B-1", "5.00"))
	env.DeliverOrders(t)

	after := broadcastRevenue(t, c, id)
	assert.EqualValues(t, 1, after.Orders, "a refund is not another order")
	assert.EqualValues(t, 1500, after.Amount)
	assert.Equal(t, after, broadcastListRevenue(t, c, id))
	_, days = byDay(t, env)
	assert.EqualValues(t, 2000, days[reportDay(3)], "the day of the sale is left alone")
	assert.EqualValues(t, -500, days[reportDay(1)])
}

func TestAnAutomationShowsItsOrdersAndNetRevenueOnItsReportItsListAndItsRead(t *testing.T) {
	env := setupCredit(t)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)
	s := newShop(t, env)
	id := int64(fixtures.AutomationWelcomeSeriesID)
	clicked(t, env, events.EmailEngagement{AutomationID: fixtures.AutomationWelcomeSeriesID}, noonDaysAgo(4))

	s.at(noonDaysAgo(3), "Order Completed", orderProps("A-1", "40.00"))
	s.at(noonDaysAgo(2), "Order Cancelled", orderProps("A-1", "0"))
	s.at(noonDaysAgo(3), "Order Completed", orderProps("A-2", "10.00"))
	env.DeliverOrders(t)

	report, listed, got := automationRevenue(t, c, id)
	assert.EqualValues(t, 2, report.Orders)
	assert.EqualValues(t, 1000, report.Amount)
	assert.Equal(t, report, listed)
	assert.Equal(t, report, got)
}

func TestBeforeTheFirstOrderThereIsNoFigureAndItsOrderCountsAtOnce(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)
	s := newShop(t, env)
	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID}, noonDaysAgo(4))
	unset := broadcastRevenue(t, c, int64(fixtures.BroadcastSentUntrackedID))

	s.at(noonDaysAgo(3), "Order Completed", orderProps("B-1", "20.00"))
	env.DeliverOrders(t)

	set := broadcastRevenue(t, c, int64(fixtures.BroadcastSentUntrackedID))
	assert.True(t, unset.ReportingCurrency.Null)
	assert.Zero(t, unset.Orders)
	assert.Equal(t, siteapi.NewNilCurrencyCode("USD"), set.ReportingCurrency)
	assert.EqualValues(t, 1, set.Orders)
	assert.EqualValues(t, 2000, set.Amount)
}

func TestAnUnattributedOrderIsCreditedToNoBroadcastAndNoAutomation(t *testing.T) {
	env := setupCredit(t)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)
	newShop(t, env).at(noonDaysAgo(3), "Order Completed", orderProps("U-1", "20.00"))
	env.DeliverOrders(t)

	assert.Zero(t, broadcastRevenue(t, c, int64(fixtures.BroadcastSentUntrackedID)).Amount)
	report, _, _ := automationRevenue(t, c, int64(fixtures.AutomationWelcomeSeriesID))
	assert.Zero(t, report.Amount)
}

// The overview splits the revenue credited to messages from the total: an Order placed before
// the click is in the total only.
func TestTheOverviewCountsThePartOfRevenueCreditedToMessages(t *testing.T) {
	env := setupCredit(t)
	s := newShop(t, env)
	clicked(t, env, events.EmailEngagement{BroadcastID: fixtures.BroadcastSentUntrackedID}, noonDaysAgo(4))
	s.at(noonDaysAgo(3), "Order Completed", orderProps("B-1", "20.00"))
	s.at(noonDaysAgo(10), "Order Completed", orderProps("U-1", "7.25"))
	env.DeliverOrders(t)

	got := revenue(t, env.SiteActor(t, fixtures.MemberMaryEmail), siteapi.OptSiteAnalyticsRevenueRange{})

	assert.EqualValues(t, 2725, got.Total)
	assert.EqualValues(t, 2, got.Orders)
	assert.EqualValues(t, 2000, got.Credited)
	assert.EqualValues(t, 1, got.CreditedOrders)
}
