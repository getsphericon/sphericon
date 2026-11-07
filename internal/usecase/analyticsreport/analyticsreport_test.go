package analyticsreport_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/analyticsreport"
)

func overviewAt(t *testing.T, env *testhelper.TestEnv, ws int64, w analyticsreport.Window, now time.Time) *analyticsreport.Overview {
	t.Helper()
	m := analyticsreport.New(func() time.Time { return now }, 0)
	ov, err := m.Overview(context.Background(), env.DB.Scoped(ws), w)
	require.NoError(t, err)
	return ov
}

func sentPerDay(ov *analyticsreport.Overview) map[string]int {
	days := map[string]int{}
	for _, p := range ov.Series {
		days[p.Date] = p.Sent
	}
	return days
}

func overview(t *testing.T, ws int64, w analyticsreport.Window) *analyticsreport.Overview {
	t.Helper()
	env := testhelper.Setup(t)
	ov, err := analyticsreport.New(time.Now, 0).Overview(context.Background(), env.DB.Scoped(ws), w)
	require.NoError(t, err)
	return ov
}

// Fixtures (outbound_messages.yml): Acme sent 11 Broadcast messages (5 opened, 2 clicked) three
// days ago, 10 (4, 1) nine days ago, 10 (5, 2) sixteen days ago, 9 (4, 1) twenty-four days ago, 10
// (6, 3) forty days ago and 10 (4, 1) seventy days ago. Four more unengaged marketing messages went
// out in the last 30 hours (3 Broadcast ones and 1 Automation one) and are in every window's cohort.
func TestEngagementIsCountedOverTheSendCohortOfTheWindow(t *testing.T) {
	cases := []struct {
		window                analyticsreport.Window
		sent, opened, clicked int
	}{
		{analyticsreport.Window7Days, 15, 5, 2},
		{analyticsreport.Window30Days, 44, 18, 6},
		{analyticsreport.Window90Days, 64, 28, 10},
	}
	for _, c := range cases {
		ov := overview(t, fixtures.AcmeID, c.window)
		assert.Equal(t, c.sent, ov.Current.Email.Sent, c.window)
		assert.Equal(t, c.opened, ov.Current.Email.Opened, c.window)
		assert.Equal(t, c.clicked, ov.Current.Email.Clicked, c.window)
		assert.InDelta(t, float64(c.opened)/float64(c.sent), ov.Current.Email.OpenRate, 1e-6)
		assert.InDelta(t, float64(c.clicked)/float64(c.sent), ov.Current.Email.ClickRate, 1e-6)
		assert.InDelta(t, float64(c.clicked)/float64(c.opened), ov.Current.Email.ClickToOpenRate, 1e-6)
		assert.LessOrEqual(t, ov.Current.Email.Opened, ov.Current.Email.Sent)
		assert.LessOrEqual(t, ov.Current.Email.Clicked, ov.Current.Email.Opened)
		assert.LessOrEqual(t, ov.Current.Email.OpenRate, float32(1))
	}
}

// Fixtures (outbound_messages.yml), Acme over the last 7 days: 14 Broadcast messages (11 three days
// ago, of which 3 delivered, 1 of them bounced and 1 complained, and 3 in the last 30 hours), 1
// Automation message 20 hours ago and 6 Transactional ones one to four days ago. The send of 8 days
// ago and the older ones are outside it, as are the messages never sent (pending, skipped, failed).
func TestSendingCountsEveryMessageSentInTheWindowBySurface(t *testing.T) {
	ov := overview(t, fixtures.AcmeID, analyticsreport.Window7Days)

	assert.Equal(t, analyticsreport.Sending{
		Broadcast: 14, Automation: 1, Transactional: 6,
		Delivered: 3, Bounced: 1, Complained: 1,
	}, ov.Current.Sending)
}

func TestTimeSeriesIsOneZeroFilledUTCDayPerDayEndingToday(t *testing.T) {
	ov := overview(t, fixtures.AcmeID, analyticsreport.Window7Days)
	require.Len(t, ov.Series, 7)

	today := time.Now().UTC().Truncate(24 * time.Hour)
	sent := 0
	for i, p := range ov.Series {
		assert.Equal(t, today.AddDate(0, 0, i-6).Format("2006-01-02"), p.Date)
		sent += p.Sent
	}
	assert.Equal(t, ov.Current.Email.Sent, sent, "the series reconciles with the cards")

	threeDaysAgo := ov.Series[3]
	assert.Equal(t, 11, threeDaysAgo.Sent)
	assert.Equal(t, 5, threeDaysAgo.Opened)
	assert.Equal(t, 2, threeDaysAgo.Clicked)
	assert.Equal(t, analyticsreport.Point{Date: ov.Series[0].Date}, ov.Series[0], "an empty day is all zeros")
}

// An Automation message is in the engagement cohort like a Broadcast one (ADR 0053): its first
// human open and click, stamped on the message, reach the cards and the series. A Transactional
// message is not in the cohort, tracked or not.
func TestAnAutomationMessageIsCountedAndATransactionalOneIsNot(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	read := func() *analyticsreport.Overview {
		ov, err := analyticsreport.New(time.Now, 0).Overview(ctx, acme, analyticsreport.Window7Days)
		require.NoError(t, err)
		return ov
	}
	before := read()

	for _, id := range []int64{fixtures.OutboundMessageAutomationSentRecentlyID, fixtures.OutboundMessageWelcomeID} {
		for _, record := range []func(context.Context, *ent.Scoped, int64, time.Time) (bool, error){outboundmessages.StampOpened, outboundmessages.StampClicked} {
			set, err := record(ctx, acme, id, time.Now())
			require.NoError(t, err)
			require.True(t, set)
		}
	}
	after := read()

	assert.Equal(t, before.Current.Email.Sent, after.Current.Email.Sent)
	assert.Equal(t, before.Current.Email.Opened+1, after.Current.Email.Opened, "only the Automation open")
	assert.Equal(t, before.Current.Email.Clicked+1, after.Current.Email.Clicked, "only the Automation click")
	assert.LessOrEqual(t, after.Current.Email.OpenRate, float32(1))

	opened, clicked := 0, 0
	for i, p := range after.Series {
		opened += p.Opened - before.Series[i].Opened
		clicked += p.Clicked - before.Series[i].Clicked
	}
	assert.Equal(t, 1, opened, "the series carries the Automation open")
	assert.Equal(t, 1, clicked, "the series carries the Automation click")
}

func TestSeriesLengthFollowsTheWindow(t *testing.T) {
	assert.Len(t, overview(t, fixtures.AcmeID, analyticsreport.Window30Days).Series, 30)
	assert.Len(t, overview(t, fixtures.AcmeID, analyticsreport.Window90Days).Series, 90)
}

func TestContactsSplitByTheGlobalOptOutPredicate(t *testing.T) {
	ov := overview(t, fixtures.AcmeID, analyticsreport.Window30Days)
	assert.Positive(t, ov.Contacts.Total)
	assert.Positive(t, ov.Contacts.Unsubscribed, "suppressed and opted-out contacts are non-mailable")
	assert.Equal(t, ov.Contacts.Total, ov.Contacts.Active+ov.Contacts.Unsubscribed)
	assert.LessOrEqual(t, ov.Current.NewContacts, ov.Contacts.Total)
}

func TestAutomationsSnapshot(t *testing.T) {
	ov := overview(t, fixtures.AcmeID, analyticsreport.Window30Days)
	assert.Positive(t, ov.Automations.Total)
	assert.LessOrEqual(t, ov.Automations.Active, ov.Automations.Total)
}

func TestAnEmptyCohortHasZeroRates(t *testing.T) {
	ov := overview(t, fixtures.AcmeID, analyticsreport.Window7Days)
	empty := ov.Series[0]
	require.Zero(t, empty.Sent)

	// Globex has one marketing message in the window (a usage fixture) and none of it engaged.
	other := overview(t, fixtures.GlobexID, analyticsreport.Window7Days)
	assert.Equal(t, 1, other.Current.Email.Sent)
	assert.Zero(t, other.Current.Email.Opened)
	assert.Zero(t, other.Current.Email.OpenRate)
	assert.Zero(t, other.Current.Email.ClickToOpenRate)
}

// An archived Automation is out of the counts, active or not.
func TestArchivedAutomationsAreNotCounted(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	before, err := analyticsreport.New(time.Now, 0).Overview(ctx, acme, analyticsreport.Window30Days)
	require.NoError(t, err)
	require.NoError(t, acme.Automation().Archive(ctx, fixtures.AutomationWelcomeSeriesID))

	after, err := analyticsreport.New(time.Now, 0).Overview(ctx, acme, analyticsreport.Window30Days)
	require.NoError(t, err)
	assert.Equal(t, before.Automations.Total-1, after.Automations.Total)
	assert.Equal(t, before.Automations.Active-1, after.Automations.Active)
}

// Fixtures (Gotham, America/New_York): sends at 2026-03-08 04:59Z (23:59 EST on 7 March), 05:00Z
// (midnight EST, opened and clicked), 2026-03-09 03:59Z (23:59 EDT on 8 March) and 04:00Z (9 March);
// and at 2026-11-01 03:59Z (23:59 EDT on 31 October), 04:00Z (opened), 2026-11-02 04:59Z (23:59 EST
// on 1 November, opened and clicked) and 05:00Z (2 November).
func TestTheSpringForwardDayIsOneTwentyThreeHourBucket(t *testing.T) {
	env := testhelper.Setup(t)
	ov := overviewAt(t, env, fixtures.GothamID, analyticsreport.Window7Days, time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC))

	want := map[string]int{
		"2026-03-04": 0, "2026-03-05": 0, "2026-03-06": 0, "2026-03-07": 1,
		"2026-03-08": 2, "2026-03-09": 1, "2026-03-10": 0,
	}
	assert.Equal(t, want, sentPerDay(ov))
	assert.Len(t, ov.Series, 7, "no day is skipped or doubled")
	assert.Equal(t, 4, ov.Current.Email.Sent, "the cards and the series agree on the window")
}

func TestTheFallBackDayIsOneTwentyFiveHourBucket(t *testing.T) {
	env := testhelper.Setup(t)
	ov := overviewAt(t, env, fixtures.GothamID, analyticsreport.Window7Days, time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC))

	want := map[string]int{
		"2026-10-28": 0, "2026-10-29": 0, "2026-10-30": 0, "2026-10-31": 1,
		"2026-11-01": 2, "2026-11-02": 1, "2026-11-03": 0,
	}
	assert.Equal(t, want, sentPerDay(ov))
	assert.Len(t, ov.Series, 7, "no day is skipped or doubled")
	assert.Equal(t, 2, ov.Series[4].Opened)
	assert.Equal(t, 1, ov.Series[4].Clicked)
}

func TestAZoneChangeMovesTheSameSendsToOtherDays(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	inNewYork := sentPerDay(overviewAt(t, env, fixtures.GothamID, analyticsreport.Window7Days, now))

	require.NoError(t, env.DB.Workspace.UpdateOneID(fixtures.GothamID).SetTimezone("UTC").Exec(context.Background()))
	inUTC := sentPerDay(overviewAt(t, env, fixtures.GothamID, analyticsreport.Window7Days, now))

	assert.Equal(t, 1, inNewYork["2026-03-07"])
	assert.Equal(t, 0, inUTC["2026-03-07"])
	assert.Equal(t, 2, inUTC["2026-03-08"])
	assert.Equal(t, 2, inUTC["2026-03-09"])
}

func TestTodayIsTheLocalDayNotTheUTCDay(t *testing.T) {
	env := testhelper.Setup(t)
	// 02:00 UTC on 9 March is still 8 March (22:00 EDT) in New York.
	ov := overviewAt(t, env, fixtures.GothamID, analyticsreport.Window7Days, time.Date(2026, 3, 9, 2, 0, 0, 0, time.UTC))

	assert.Equal(t, "2026-03-08", ov.Series[6].Date)
	assert.Equal(t, 2, ov.Series[6].Sent)
}

// Read on 14 March over 7 days, Gotham's current period is 8-14 March in New York and the previous
// one 1-7 March. Send A and the manual sign-up at 23:59 EST on 7 March (8 March in UTC) fall in the
// previous period; sends B (opened and clicked), C and D and the four later sign-ups in the current one.
func TestEachPeriodFigureIsReadForThePeriodJustBeforeTheWindowToo(t *testing.T) {
	env := testhelper.Setup(t)

	ov := overviewAt(t, env, fixtures.GothamID, analyticsreport.Window7Days, time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC))

	assert.Equal(t, analyticsreport.Period{
		From: "2026-03-08", To: "2026-03-14",
		Sending:     analyticsreport.Sending{Broadcast: 3},
		Email:       analyticsreport.Email{Sent: 3, Opened: 1, Clicked: 1, OpenRate: 1.0 / 3, ClickRate: 1.0 / 3, ClickToOpenRate: 1},
		NewContacts: 4,
	}, ov.Current)
	assert.Equal(t, analyticsreport.Period{
		From: "2026-03-01", To: "2026-03-07",
		Sending:     analyticsreport.Sending{Broadcast: 1},
		Email:       analyticsreport.Email{Sent: 1},
		NewContacts: 1,
	}, ov.Previous)
	assert.Len(t, ov.Series, 7, "the series is the current period's")
}

// Read on 14 March over 7 days (Gotham, USD): the current period has the USD Order completed on 12
// March (50.00), its 10.00 refund of 13 March and an Order in EUR the same day, counted in no sum; the
// previous one the Order of 5 March (25.00), whose refund of 15 March is after both. Every one of
// them is credited to Gotham results.
func TestRevenueIsReadForThePeriodJustBeforeTheWindowToo(t *testing.T) {
	env := testhelper.Setup(t)
	m := analyticsreport.New(func() time.Time { return time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC) }, 0)

	report, err := m.Revenue(context.Background(), env.DB.Scoped(fixtures.GothamID), analyticsreport.Window7Days)
	require.NoError(t, err)

	assert.Equal(t, orders.RevenuePeriod{
		From: "2026-03-08", To: "2026-03-14",
		Total: 4000, Credited: 4000, Orders: 1, CreditedOrders: 1,
		OtherCurrencyOrders: []orders.CurrencyOrders{{Currency: "EUR", Orders: 1}},
	}, report.Current)
	assert.Equal(t, orders.RevenuePeriod{
		From: "2026-03-01", To: "2026-03-07",
		Total: 2500, Credited: 2500, Orders: 1, CreditedOrders: 1, OtherCurrencyOrders: []orders.CurrencyOrders{},
	}, report.Previous)
	assert.Len(t, report.Series, 7, "the series is the current period's")
}
