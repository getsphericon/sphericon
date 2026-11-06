package site_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Orders and Revenue of a Broadcast and of an Automation (ADR 0064): read from the Movements of
// the Orders credited to it, in the Reporting currency, never stored. Which Order is credited to
// which is the orders module's, tested there; these tests read what the reports sum, over the
// Orders of the revenue tenants (fixtures/orders.yml), one tenant per report.

func broadcastRevenue(t *testing.T, c *siteapi.Client, ws siteapi.WorkspaceId, id int64) siteapi.SiteMessageRevenue {
	t.Helper()
	res, err := c.SiteBroadcastsGet(t.Context(), siteapi.SiteBroadcastsGetParams{WorkspaceId: ws, ID: idStr(id)})
	require.NoError(t, err)
	b := res
	return b.Stats.Revenue
}

func broadcastListRevenue(t *testing.T, c *siteapi.Client, ws siteapi.WorkspaceId, id int64) siteapi.SiteMessageRevenue {
	t.Helper()
	res, err := c.SiteBroadcastsList(t.Context(), siteapi.SiteBroadcastsListParams{WorkspaceId: ws, PageSize: siteapi.NewOptInt32(100)})
	require.NoError(t, err)
	list := res
	for i := range list.Items {
		if int64(list.Items[i].ID) == id {
			return list.Items[i].Stats.Revenue
		}
	}
	require.FailNow(t, "broadcast not listed")
	return siteapi.SiteMessageRevenue{}
}

func automationRevenue(t *testing.T, c *siteapi.Client, ws siteapi.WorkspaceId, id int64) (report, listed, got siteapi.SiteMessageRevenue) {
	t.Helper()
	rep, err := c.SiteAutomationReportsGet(t.Context(), siteapi.SiteAutomationReportsGetParams{WorkspaceId: ws, AutomationId: idStr(id)})
	require.NoError(t, err)
	r := rep
	lst, err := c.SiteAutomationsList(t.Context(), siteapi.SiteAutomationsListParams{WorkspaceId: ws, PageSize: siteapi.NewOptInt32(100)})
	require.NoError(t, err)
	l := lst
	for i := range l.Items {
		if int64(l.Items[i].ID) == id {
			listed = l.Items[i].Revenue.Value
		}
	}
	one, err := c.SiteAutomationsGet(t.Context(), siteapi.SiteAutomationsGetParams{WorkspaceId: ws, ID: idStr(id)})
	require.NoError(t, err)
	a := one
	return r.Revenue, listed, a.Revenue.Value
}

// Bazaar's Broadcast earned B-1, 20.00 three days ago, refunded 5.00 a day ago, and B-2 in EUR.
func TestABroadcastShowsItsOrdersAndNetRevenueAndARefundLowersItOnItsDay(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.RevenueOwnerRitaEmail)
	id := int64(fixtures.BroadcastBazaarID)

	got := broadcastRevenue(t, c, fixtures.BazaarPublicID, id)

	assert.Equal(t, siteapi.NewNilCurrencyCode("USD"), got.ReportingCurrency)
	assert.EqualValues(t, 1, got.Orders, "a refund is not another order, and the Order in EUR is counted in no sum")
	assert.EqualValues(t, 1500, got.Amount)
	assert.Equal(t, got, broadcastListRevenue(t, c, fixtures.BazaarPublicID, id))
	_, days := byDayOf(t, c, fixtures.BazaarPublicID)
	assert.EqualValues(t, 2000, days[reportDay(3)], "the day of the sale is left alone")
	assert.EqualValues(t, -500, days[reportDay(1)])
}

// Emporium's revenue Automation earned A-1, 40.00 cancelled the next day, and A-2, 10.00.
func TestAnAutomationShowsItsOrdersAndNetRevenueOnItsReportItsListAndItsRead(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.RevenueOwnerRitaEmail)

	report, listed, got := automationRevenue(t, c, fixtures.EmporiumPublicID, fixtures.AutomationEmporiumRevenueID)

	assert.EqualValues(t, 2, report.Orders)
	assert.EqualValues(t, 1000, report.Amount)
	assert.Equal(t, report, listed)
	assert.Equal(t, report, got)
}

// Souk has no Order and no Reporting currency; its Contact clicked its Broadcast four days ago.
func TestBeforeTheFirstOrderThereIsNoFigureAndItsOrderCountsAtOnce(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.RevenueOwnerRitaEmail)
	unset := broadcastRevenue(t, c, fixtures.SoukPublicID, fixtures.BroadcastSoukID)

	in := orderInput(fixtures.ContactSoukBuyerEmail, "Order Completed", orderProps("S-1", "20.00"))
	in.Email = externalapi.NewOptNilEmailAddress(fixtures.ContactSoukBuyerEmail)
	in.OccurredAt = externalapi.NewOptNilTimestamp(externalapi.Timestamp(noonDaysAgo(3)))
	shop := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.SoukID, "events:write"))
	_, err := shop.EventsBatchSubmit(t.Context(), &externalapi.RecordEventsBatchInput{Backfill: externalapi.NewOptBool(true), Events: []externalapi.EventInput{in}})
	require.NoError(t, err)

	env.PerformEnqueuedJobs(t, jobs.OrderEventArgs{})

	set := broadcastRevenue(t, c, fixtures.SoukPublicID, fixtures.BroadcastSoukID)
	assert.True(t, unset.ReportingCurrency.Null)
	assert.Zero(t, unset.Orders)
	assert.Equal(t, siteapi.NewNilCurrencyCode("USD"), set.ReportingCurrency)
	assert.EqualValues(t, 1, set.Orders)
	assert.EqualValues(t, 2000, set.Amount)
}

func TestAnUnattributedOrderIsCreditedToNoBroadcastAndNoAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	chooseCurrency(t, env.SiteActor(t, fixtures.OwnerJohnEmail), "USD")
	c := env.SiteActor(t, fixtures.MemberMaryEmail)
	newShop(t, env).at(noonDaysAgo(3), "Order Completed", orderProps("U-1", "20.00"))
	env.PerformEnqueuedJobs(t, jobs.OrderEventArgs{})

	assert.Zero(t, broadcastRevenue(t, c, fixtures.AcmePublicID, int64(fixtures.BroadcastSentUntrackedID)).Amount)
	report, _, _ := automationRevenue(t, c, fixtures.AcmePublicID, int64(fixtures.AutomationWelcomeSeriesID))
	assert.Zero(t, report.Amount)
}

// The overview splits the revenue credited to messages from the total: Mercado's U-1, credited to
// no message, is in the total only.
func TestTheOverviewCountsThePartOfRevenueCreditedToMessages(t *testing.T) {
	env := testhelper.Setup(t)

	got := revenueOf(t, env.SiteActor(t, fixtures.RevenueOwnerRitaEmail), fixtures.MercadoPublicID, siteapi.OptSiteAnalyticsRevenueRange{})

	assert.EqualValues(t, 2725, got.Current.Total)
	assert.EqualValues(t, 2, got.Current.Orders)
	assert.EqualValues(t, 2000, got.Current.Credited)
	assert.EqualValues(t, 1, got.Current.CreditedOrders)
}
