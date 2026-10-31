package site_test

import (
	"testing"
	"time"

	"github.com/go-faster/jx"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/order"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// shop is the Workspace's shop: it sends order Events through the external API.
type shop struct {
	t   *testing.T
	api *externalapi.Client
}

func newShop(t *testing.T, env *testhelper.TestEnv) shop {
	t.Helper()
	return shop{t: t, api: env.ExternalScoped(t, "events:write", "contacts:erase")}
}

func orderInput(subject, action string, props map[string]string) externalapi.EventInput {
	raw := make(externalapi.JsonObject, len(props))
	for k, v := range props {
		raw[k] = jx.Raw(v)
	}
	return externalapi.EventInput{
		SubjectId:  subject,
		Email:      externalapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail),
		Action:     action,
		Properties: externalapi.NewOptNilJsonObject(raw),
	}
}

// completed sends an Order Completed of the known Contact Alice.
func (s shop) completed(id, total, currency string) {
	s.t.Helper()
	s.send(orderInput("alice", "Order Completed", map[string]string{"order_id": `"` + id + `"`, "total": total, "currency": `"` + currency + `"`}))
}

func (s shop) send(in externalapi.EventInput) {
	s.t.Helper()
	res, err := s.api.EventsCreate(s.t.Context(), &externalapi.RecordEventsInput{Events: []externalapi.EventInput{in}})
	require.NoError(s.t, err)
	require.IsType(s.t, &externalapi.EventsCreateNoContent{}, res)
}

func revenue(t *testing.T, c *siteapi.Client, rng siteapi.OptSiteAnalyticsRevenueRange) *siteapi.SiteAnalyticsRevenue {
	t.Helper()
	res, err := c.SiteAnalyticsRevenue(t.Context(), siteapi.SiteAnalyticsRevenueParams{WorkspaceId: fixtures.AcmePublicID, Range: rng})
	require.NoError(t, err)
	got, ok := res.(*siteapi.SiteAnalyticsRevenue)
	require.Truef(t, ok, "got %T", res)
	return got
}

func chooseCurrency(t *testing.T, c *siteapi.Client, code string) {
	t.Helper()
	res, err := c.SiteWorkspacesUpdateRevenueSettings(t.Context(),
		&siteapi.SiteUpdateRevenueSettingsInput{ReportingCurrency: siteapi.NewOptCurrencyCode(siteapi.CurrencyCode(code))},
		siteapi.SiteWorkspacesUpdateRevenueSettingsParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteRevenueSettingsResource{}, res)
}

func TestACompletedOrderIsRevenueInTheMinorUnitOfTheReportingCurrency(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	chooseCurrency(t, owner, "USD")

	newShop(t, env).completed("A-1", "19.99", "USD")
	env.DeliverOrders(t)

	got := revenue(t, env.SiteActor(t, fixtures.MemberMaryEmail), siteapi.OptSiteAnalyticsRevenueRange{})
	assert.Equal(t, siteapi.CurrencyCode("USD"), got.ReportingCurrency.Value)
	assert.EqualValues(t, 1999, got.Total)
	assert.EqualValues(t, 1, got.Orders)
	assert.Len(t, got.Series, 30, "the range is 30 Report days until another is asked for")
	today := got.Series[len(got.Series)-1]
	assert.EqualValues(t, 1999, today.Amount)
	assert.EqualValues(t, 1, today.Orders)
}

func TestYenAreWholeUnitsAndTheRangeCanBeSevenDays(t *testing.T) {
	env := testhelper.Setup(t)
	chooseCurrency(t, env.SiteActor(t, fixtures.OwnerJohnEmail), "JPY")

	newShop(t, env).completed("A-1", "1500", "JPY")
	env.DeliverOrders(t)

	got := revenue(t, env.SiteActor(t, fixtures.MemberMaryEmail), siteapi.NewOptSiteAnalyticsRevenueRange(siteapi.SiteAnalyticsRevenueRange7d))
	assert.EqualValues(t, 1500, got.Total)
	assert.Len(t, got.Series, 7)
}

func TestARepeatOfTheSameOrderIdChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	chooseCurrency(t, env.SiteActor(t, fixtures.OwnerJohnEmail), "USD")
	s := newShop(t, env)

	s.completed("A-1", "10.00", "USD")
	s.completed("A-1", "99.00", "USD")
	env.DeliverOrders(t)
	env.DeliverOrders(t) // at-least-once delivery: the whole outbox again

	got := revenue(t, env.SiteActor(t, fixtures.MemberMaryEmail), siteapi.OptSiteAnalyticsRevenueRange{})
	assert.EqualValues(t, 1000, got.Total)
	assert.EqualValues(t, 1, got.Orders)
}

func TestRevenueIsUnsetUntilTheCurrencyIsChosenThenCountsEveryOrderAlreadyReceived(t *testing.T) {
	env := testhelper.Setup(t)
	s := newShop(t, env)
	s.completed("A-1", "10.00", "EUR")
	s.completed("A-2", "5.00", "EUR")
	env.DeliverOrders(t)
	member := env.SiteActor(t, fixtures.MemberMaryEmail)

	before := revenue(t, member, siteapi.OptSiteAnalyticsRevenueRange{})
	chooseCurrency(t, env.SiteActor(t, fixtures.AdminAliceEmail), "EUR")
	after := revenue(t, member, siteapi.OptSiteAnalyticsRevenueRange{})

	assert.True(t, before.ReportingCurrency.Null)
	assert.EqualValues(t, 0, before.Total)
	assert.EqualValues(t, 0, before.Orders)
	assert.EqualValues(t, 1500, after.Total)
	assert.EqualValues(t, 2, after.Orders)
}

func TestAnOrderInAnotherCurrencyIsStoredAndCountedInNoSum(t *testing.T) {
	env := testhelper.Setup(t)
	chooseCurrency(t, env.SiteActor(t, fixtures.OwnerJohnEmail), "USD")
	s := newShop(t, env)

	s.completed("A-1", "10.00", "USD")
	s.completed("A-2", "500", "JPY")
	env.DeliverOrders(t)

	got := revenue(t, env.SiteActor(t, fixtures.MemberMaryEmail), siteapi.OptSiteAnalyticsRevenueRange{})
	assert.EqualValues(t, 1000, got.Total)
	assert.EqualValues(t, 1, got.Orders)
	assert.EqualValues(t, 1, got.OtherCurrencyOrders)
	kept := env.DB.Scoped(fixtures.AcmeID).Order().Query().Where(order.OrderID("A-2")).OnlyX(t.Context())
	assert.Equal(t, "JPY", kept.Currency)
}

func TestAnOrderWithoutACurrencyCountsInTheReportingCurrencyAndKeepsItAfterAChange(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	chooseCurrency(t, owner, "EUR")
	newShop(t, env).send(orderInput("alice", "Order Completed", map[string]string{"order_id": `"A-1"`, "total": `10.50`}))
	env.DeliverOrders(t)

	inEUR := revenue(t, owner, siteapi.OptSiteAnalyticsRevenueRange{})
	chooseCurrency(t, owner, "JPY")
	inJPY := revenue(t, owner, siteapi.OptSiteAnalyticsRevenueRange{})

	assert.EqualValues(t, 1050, inEUR.Total)
	assert.EqualValues(t, 0, inJPY.Total, "an Order keeps the currency it was read in: it is now another currency")
	assert.EqualValues(t, 1, inJPY.OtherCurrencyOrders)
}

func TestAnOrderOfAnUnknownContactCountsAndCreatesNoContact(t *testing.T) {
	env := testhelper.Setup(t)
	chooseCurrency(t, env.SiteActor(t, fixtures.OwnerJohnEmail), "USD")

	in := orderInput("guest-checkout-1", "Order Completed", map[string]string{"order_id": `"G-1"`, "total": `7.25`, "currency": `"USD"`})
	in.Email = externalapi.OptNilEmailAddress{}
	newShop(t, env).send(in)
	env.DeliverOrders(t)

	got := revenue(t, env.SiteActor(t, fixtures.MemberMaryEmail), siteapi.OptSiteAnalyticsRevenueRange{})
	assert.EqualValues(t, 725, got.Total)
	assert.Equal(t, 0, env.OutboxCount(t, "contact.created", map[string]any{"email": ""}), "no Contact is created")
	guest := env.DB.Scoped(fixtures.AcmeID).Order().Query().Where(order.OrderID("G-1")).OnlyX(t.Context())
	assert.Nil(t, guest.ContactID)
}

func TestErasingTheContactUnlinksItsOrdersAndTheAmountsStay(t *testing.T) {
	env := testhelper.Setup(t)
	chooseCurrency(t, env.SiteActor(t, fixtures.OwnerJohnEmail), "USD")
	s := newShop(t, env)
	s.completed("A-1", "20.00", "USD")
	env.DeliverOrders(t)
	linked := env.DB.Scoped(fixtures.AcmeID).Order().Query().Where(order.OrderID("A-1")).OnlyX(t.Context())
	require.NotNil(t, linked.ContactID)

	res, err := s.api.ContactsEraseBy(t.Context(), externalapi.ContactsEraseByParams{Email: externalapi.NewOptEmailAddress(fixtures.ContactAliceEmail)})
	require.NoError(t, err)
	require.IsType(t, &externalapi.ContactsEraseByNoContent{}, res)

	kept := env.DB.Scoped(fixtures.AcmeID).Order().Query().Where(order.OrderID("A-1")).OnlyX(t.Context())
	got := revenue(t, env.SiteActor(t, fixtures.MemberMaryEmail), siteapi.OptSiteAnalyticsRevenueRange{})
	gone := env.DB.Scoped(fixtures.AcmeID).Contact().Query().Where(contact.Email(fixtures.ContactAliceEmail)).ExistX(t.Context())
	assert.False(t, gone)
	assert.Nil(t, kept.ContactID)
	assert.EqualValues(t, 2000, got.Total)
}

func TestABackfilledHistoryBuildsOrdersOnTheDaysTheyHappened(t *testing.T) {
	env := testhelper.Setup(t)
	chooseCurrency(t, env.SiteActor(t, fixtures.OwnerJohnEmail), "USD")
	old := externalapi.Timestamp(time.Now().AddDate(0, 0, -40))
	in := orderInput("alice", "Order Completed", map[string]string{"order_id": `"H-1"`, "total": `30.00`, "currency": `"USD"`})
	in.OccurredAt = externalapi.NewOptNilTimestamp(old)

	res, err := newShop(t, env).api.EventsBatchSubmit(t.Context(), &externalapi.RecordEventsBatchInput{Backfill: externalapi.NewOptBool(true), Events: []externalapi.EventInput{in}})
	require.NoError(t, err)
	require.IsType(t, &externalapi.RecordEventsBatchResult{}, res)
	env.DeliverOrders(t)

	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	thirty := revenue(t, member, siteapi.OptSiteAnalyticsRevenueRange{})
	ninety := revenue(t, member, siteapi.NewOptSiteAnalyticsRevenueRange(siteapi.SiteAnalyticsRevenueRange90d))
	assert.EqualValues(t, 0, thirty.Total)
	assert.EqualValues(t, 3000, ninety.Total)
	day, found := lo.Find(ninety.Series, func(p siteapi.SiteRevenuePoint) bool { return p.Amount != 0 })
	require.True(t, found)
	assert.Equal(t, time.Time(old).UTC().Format("2006-01-02"), day.Date, "UTC Workspace: the Report day is the UTC date")
}

func TestAResentBackfillLoadChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	chooseCurrency(t, env.SiteActor(t, fixtures.OwnerJohnEmail), "USD")
	api := newShop(t, env).api
	load := func(clientEventID, orderID string) {
		in := orderInput("alice", "Order Completed", map[string]string{"order_id": `"` + orderID + `"`, "total": `30.00`, "currency": `"USD"`})
		in.OccurredAt = externalapi.NewOptNilTimestamp(externalapi.Timestamp(time.Now().AddDate(0, 0, -40)))
		in.ClientEventId = externalapi.NewOptNilString(clientEventID)
		res, err := api.EventsBatchSubmit(t.Context(), &externalapi.RecordEventsBatchInput{Backfill: externalapi.NewOptBool(true), Events: []externalapi.EventInput{in}})
		require.NoError(t, err)
		require.IsType(t, &externalapi.RecordEventsBatchResult{}, res)
		env.DeliverOrders(t)
	}

	load("ce-1", "H-1")
	load("ce-1", "H-1") // the same load resent: the clientEventId dedups it
	load("ce-2", "H-1") // a new Event for the same order: the order_id rule makes it a no-op

	got := revenue(t, env.SiteActor(t, fixtures.MemberMaryEmail), siteapi.NewOptSiteAnalyticsRevenueRange(siteapi.SiteAnalyticsRevenueRange90d))
	assert.EqualValues(t, 3000, got.Total)
	assert.EqualValues(t, 1, got.Orders)
}

func TestALiveOrderEventOlderThanTheLiveWindowIsRefusedNamingBackfill(t *testing.T) {
	env := testhelper.Setup(t)
	chooseCurrency(t, env.SiteActor(t, fixtures.OwnerJohnEmail), "USD")
	in := orderInput("alice", "Order Completed", map[string]string{"order_id": `"S-1"`, "total": `30.00`, "currency": `"USD"`})
	in.OccurredAt = externalapi.NewOptNilTimestamp(externalapi.Timestamp(time.Now().AddDate(0, 0, -40)))

	res, err := newShop(t, env).api.EventsBatchSubmit(t.Context(), &externalapi.RecordEventsBatchInput{Events: []externalapi.EventInput{in}})
	require.NoError(t, err)
	batch, ok := res.(*externalapi.RecordEventsBatchResult)
	require.Truef(t, ok, "got %T", res)
	require.Len(t, batch.Results, 1)
	assert.Equal(t, externalapi.EventBatchStatusFailed, batch.Results[0].Status)
	assert.Contains(t, batch.Results[0].Error.Or(""), "backfill")
	env.DeliverOrders(t)

	got := revenue(t, env.SiteActor(t, fixtures.MemberMaryEmail), siteapi.NewOptSiteAnalyticsRevenueRange(siteapi.SiteAnalyticsRevenueRange90d))
	assert.EqualValues(t, 0, got.Total)
}

func TestOnlyAMemberOfTheWorkspaceReadsItsRevenue(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := env.SiteActor(t, fixtures.OutsiderOscarEmail).SiteAnalyticsRevenue(t.Context(),
		siteapi.SiteAnalyticsRevenueParams{WorkspaceId: fixtures.AcmePublicID})

	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAnalyticsRevenueNotFound{}, res)
}

func TestOrdersWithoutACurrencyReceivedBeforeTheChoiceAreCountedOnceItIsMade(t *testing.T) {
	env := testhelper.Setup(t)
	s := newShop(t, env)
	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	noCurrency := func(action, id, total string, extra ...string) externalapi.EventInput {
		return orderInput("alice", action, orderProps2(id, total, extra...))
	}
	s.send(noCurrency("Order Completed", "A-1", "1500"))
	s.send(noCurrency("Order Completed", "A-2", "20"))
	// A change of an Order that waits for the currency waits for its Completed too.
	s.send(noCurrency("Order Refunded", "A-2", "5"))
	env.DeliverOrders(t)
	before := revenue(t, member, siteapi.OptSiteAnalyticsRevenueRange{})

	chooseCurrency(t, env.SiteActor(t, fixtures.OwnerJohnEmail), "JPY")
	env.DeliverOrders(t)
	after := revenue(t, member, siteapi.OptSiteAnalyticsRevenueRange{})

	assert.True(t, before.ReportingCurrency.Null)
	assert.EqualValues(t, 0, before.Total)
	assert.EqualValues(t, 1500+20-5, after.Total, "the amounts are read in the minor unit of the chosen currency")
	assert.EqualValues(t, 2, after.Orders)
}

func TestAnOrderWithoutACurrencyIsReadInTheCurrencyOfTheOrderItChanges(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	chooseCurrency(t, owner, "EUR")
	s := newShop(t, env)
	s.completed("A-1", "100.00", "EUR")
	env.DeliverOrders(t)
	chooseCurrency(t, owner, "JPY") // the Workspace changes its mind; the Order stays in euro

	s.send(orderInput("alice", "Order Refunded", orderProps2("A-1", "10")))
	env.DeliverOrders(t)

	chooseCurrency(t, owner, "EUR")
	got := revenue(t, owner, siteapi.OptSiteAnalyticsRevenueRange{})
	assert.EqualValues(t, 9000, got.Total, "the refund is in the Order's euro, not in the yen of the day")
}

func TestAnOrderWithoutACurrencyKeepsItsTimeAndContactWhileItWaits(t *testing.T) {
	env := testhelper.Setup(t)
	s := newShop(t, env)
	s.send(orderInput("alice", "Order Completed", orderProps2("A-1", "10.00")))
	env.DeliverOrders(t)

	chooseCurrency(t, env.SiteActor(t, fixtures.OwnerJohnEmail), "USD")
	env.DeliverOrders(t)

	kept := env.DB.Scoped(fixtures.AcmeID).Order().Query().Where(order.OrderID("A-1")).OnlyX(t.Context())
	assert.Equal(t, "USD", kept.Currency)
	require.NotNil(t, kept.ContactID, "the Contact the Event resolved to is kept")
}

// orderProps2 is the properties of an order Event that names no currency.
func orderProps2(id, total string, extra ...string) map[string]string {
	props := map[string]string{"order_id": `"` + id + `"`, "total": total}
	for i := 0; i+1 < len(extra); i += 2 {
		props[extra[i]] = extra[i+1]
	}
	return props
}
