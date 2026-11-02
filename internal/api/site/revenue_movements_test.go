package site_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// noonDaysAgo is noon UTC, n Report days back: well inside a day, and inside the live window.
func noonDaysAgo(n int) time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC).AddDate(0, 0, -n)
}

// at backfills an order Event of Alice dated at, with the properties as raw JSON.
func (s shop) at(at time.Time, action string, props map[string]string) {
	s.t.Helper()
	in := orderInput("alice", action, props)
	in.OccurredAt = externalapi.NewOptNilTimestamp(externalapi.Timestamp(at))
	// A Backfill takes any age, so a test does not depend on the time of day it runs at.
	res, err := s.api.EventsBatchSubmit(s.t.Context(), &externalapi.RecordEventsBatchInput{Backfill: externalapi.NewOptBool(true), Events: []externalapi.EventInput{in}})
	require.NoError(s.t, err)
	require.IsType(s.t, &externalapi.RecordEventsBatchResult{}, res)
}

func orderProps(id, total string, extra ...string) map[string]string {
	props := map[string]string{"order_id": `"` + id + `"`, "total": total, "currency": `"USD"`}
	for i := 0; i+1 < len(extra); i += 2 {
		props[extra[i]] = extra[i+1]
	}
	return props
}

// byDay reads the revenue of the last 7 days as {date: amount} for the days that moved.
func byDay(t *testing.T, env *testhelper.TestEnv) (total int64, days map[string]int64) {
	t.Helper()
	got := revenue(t, env.SiteActor(t, fixtures.MemberMaryEmail), siteapi.NewOptSiteAnalyticsRevenueRange(siteapi.SiteAnalyticsRevenueRange7d))
	days = map[string]int64{}
	for _, p := range got.Series {
		if p.Amount != 0 {
			days[p.Date] = p.Amount
		}
	}
	return got.Current.Total, days
}

func reportDay(n int) string { return noonDaysAgo(n).Format("2006-01-02") }

func setup(t *testing.T) (*testhelper.TestEnv, shop) {
	t.Helper()
	env := testhelper.Setup(t)
	chooseCurrency(t, env.SiteActor(t, fixtures.OwnerJohnEmail), "USD")
	return env, newShop(t, env)
}

func TestARefundSubtractsItsTotalOnItsOwnDayAndTwoRefundsAreTwoEvents(t *testing.T) {
	env, s := setup(t)

	s.at(noonDaysAgo(3), "Order Completed", orderProps("A-1", "100.00"))
	s.at(noonDaysAgo(2), "Order Refunded", orderProps("A-1", "10.00"))
	s.at(noonDaysAgo(1), "Order Refunded", orderProps("A-1", "10.00"))
	env.DeliverOrders(t)
	env.DeliverOrders(t) // redelivery counts nothing twice, with or without a refund_id

	total, days := byDay(t, env)
	assert.EqualValues(t, 8000, total)
	assert.Equal(t, map[string]int64{reportDay(3): 10000, reportDay(2): -1000, reportDay(1): -1000}, days)
}

func TestAResentRefundWithTheSameRefundIdIsANoOpAndWithoutOneEachEventCounts(t *testing.T) {
	env, s := setup(t)
	s.at(noonDaysAgo(2), "Order Completed", orderProps("A-1", "100.00"))

	s.at(noonDaysAgo(1), "Order Refunded", orderProps("A-1", "10.00", "refund_id", `"R-1"`))
	s.at(noonDaysAgo(1), "Order Refunded", orderProps("A-1", "10.00", "refund_id", `"R-1"`))
	s.at(noonDaysAgo(1), "Order Refunded", orderProps("A-1", "10.00", "refund_id", `"R-2"`))
	s.at(noonDaysAgo(1), "Order Refunded", orderProps("A-1", "5.00"))
	s.at(noonDaysAgo(1), "Order Refunded", orderProps("A-1", "5.00"))
	env.DeliverOrders(t)

	total, _ := byDay(t, env)
	assert.EqualValues(t, 10000-1000-1000-500-500, total)
}

func TestTheSameRefundIdOnAnotherOrderIsAnotherRefund(t *testing.T) {
	env, s := setup(t)
	s.at(noonDaysAgo(2), "Order Completed", orderProps("A-1", "100.00"))
	s.at(noonDaysAgo(2), "Order Completed", orderProps("A-2", "100.00"))

	s.at(noonDaysAgo(1), "Order Refunded", orderProps("A-1", "10.00", "refund_id", `"R-1"`))
	s.at(noonDaysAgo(1), "Order Refunded", orderProps("A-2", "10.00", "refund_id", `"R-1"`))
	env.DeliverOrders(t)

	total, _ := byDay(t, env)
	assert.EqualValues(t, 18000, total)
}

func TestARefundLargerThanWhatIsLeftIsAppliedOnlyUpToIt(t *testing.T) {
	env, s := setup(t)

	s.at(noonDaysAgo(3), "Order Completed", orderProps("A-1", "30.00"))
	s.at(noonDaysAgo(2), "Order Refunded", orderProps("A-1", "25.00"))
	s.at(noonDaysAgo(1), "Order Refunded", orderProps("A-1", "25.00"))
	s.at(noonDaysAgo(1), "Order Refunded", orderProps("A-1", "25.00"))
	env.DeliverOrders(t)

	total, days := byDay(t, env)
	assert.EqualValues(t, 0, total, "net revenue of an Order is never below zero")
	assert.Equal(t, map[string]int64{reportDay(3): 3000, reportDay(2): -2500, reportDay(1): -500}, days)
}

func TestAnUpdateAddsTheDifferenceToTheNetTotalUpAndDown(t *testing.T) {
	env, s := setup(t)

	s.at(noonDaysAgo(4), "Order Completed", orderProps("A-1", "100.00"))
	s.at(noonDaysAgo(3), "Order Refunded", orderProps("A-1", "20.00"))
	s.at(noonDaysAgo(2), "Order Updated", orderProps("A-1", "130.00")) // net was 80: +50
	s.at(noonDaysAgo(1), "Order Updated", orderProps("A-1", "90.00"))  // net was 130: -40
	env.DeliverOrders(t)

	total, days := byDay(t, env)
	assert.EqualValues(t, 9000, total)
	assert.Equal(t, map[string]int64{reportDay(4): 10000, reportDay(3): -2000, reportDay(2): 5000, reportDay(1): -4000}, days)
}

func TestACancellationTakesBackWhatIsLeftOnItsDayAndLaterEventsAddNothingBelowZero(t *testing.T) {
	env, s := setup(t)

	s.at(noonDaysAgo(3), "Order Completed", orderProps("A-1", "100.00"))
	s.at(noonDaysAgo(2), "Order Refunded", orderProps("A-1", "40.00"))
	s.at(noonDaysAgo(1), "Order Cancelled", map[string]string{"order_id": `"A-1"`, "cancel_reason": `"changed mind"`})
	s.at(noonDaysAgo(1), "Order Refunded", orderProps("A-1", "10.00"))
	env.DeliverOrders(t)

	total, days := byDay(t, env)
	assert.EqualValues(t, 0, total)
	assert.Equal(t, map[string]int64{reportDay(3): 10000, reportDay(2): -4000, reportDay(1): -6000}, days)
}

func TestAnOrderEventBeforeItsCompletionIsKeptAndAppliedInTimeOrderOnArrival(t *testing.T) {
	env, s := setup(t)

	// Sent out of order: the update and the refund first, the completion last.
	s.at(noonDaysAgo(1), "Order Updated", orderProps("A-1", "80.00"))
	s.at(noonDaysAgo(2), "Order Refunded", orderProps("A-1", "30.00"))
	env.DeliverOrders(t)
	before, _ := byDay(t, env)
	s.at(noonDaysAgo(3), "Order Completed", orderProps("A-1", "100.00"))
	env.DeliverOrders(t)
	env.DeliverOrders(t)

	total, days := byDay(t, env)
	assert.EqualValues(t, 0, before, "an Order not completed counts nothing")
	// By time: 100, refund 30 -> 70, update to 80 -> +10. Applied in arrival order it would be 50.
	assert.EqualValues(t, 8000, total)
	assert.Equal(t, map[string]int64{reportDay(3): 10000, reportDay(2): -3000, reportDay(1): 1000}, days)
}

func TestAnOrderEventForAnOrderThatNeverCompletesCountsNothing(t *testing.T) {
	env, s := setup(t)

	s.at(noonDaysAgo(1), "Order Refunded", orderProps("GHOST", "30.00"))
	s.at(noonDaysAgo(1), "Order Cancelled", map[string]string{"order_id": `"GHOST"`})
	env.DeliverOrders(t)

	got := revenue(t, env.SiteActor(t, fixtures.MemberMaryEmail), siteapi.OptSiteAnalyticsRevenueRange{})
	assert.EqualValues(t, 0, got.Current.Total)
	assert.EqualValues(t, 0, got.Current.Orders)
}

func TestARefundIdThatIsNotTextOrANumberIsRefusedAtIngest(t *testing.T) {
	_, s := setup(t)

	res, err := s.api.EventsCreate(t.Context(), &externalapi.RecordEventsInput{Events: []externalapi.EventInput{
		orderInput("alice", "Order Refunded", orderProps("A-1", "1.00", "refund_id", `{"a":1}`)),
	}})

	require.NoError(t, err)
	refused, ok := res.(*externalapi.EventsCreateUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, refused.Detail.Or(""), "refund_id")
}

func TestRevenueIsBucketedByTheReportDayOfTheWorkspaceTimeZone(t *testing.T) {
	env, s := setup(t)
	setTimezone(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID, "Asia/Tokyo")
	// 16:00 UTC is 01:00 of the next day in Tokyo.
	lateUTC := noonDaysAgo(3).Add(4 * time.Hour)
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)
	require.NotEqual(t, lateUTC.Format("2006-01-02"), lateUTC.In(tokyo).Format("2006-01-02"))

	s.at(lateUTC, "Order Completed", orderProps("A-1", "100.00"))
	s.at(lateUTC.Add(-8*time.Hour), "Order Refunded", orderProps("A-1", "10.00")) // 08:00 UTC, 17:00 Tokyo the same day
	env.DeliverOrders(t)

	_, days := byDay(t, env)
	assert.Equal(t, map[string]int64{
		lateUTC.In(tokyo).Format("2006-01-02"):                     10000,
		lateUTC.Add(-8 * time.Hour).In(tokyo).Format("2006-01-02"): -1000,
	}, days)
}
