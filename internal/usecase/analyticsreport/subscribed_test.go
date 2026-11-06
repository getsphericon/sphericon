package analyticsreport_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/analyticsreport"
)

func subscribedAt(t *testing.T, env *testhelper.TestEnv, ws int64, w analyticsreport.Window, now time.Time) *analyticsreport.SubscribedPeople {
	t.Helper()
	m := analyticsreport.New().WithClock(func() time.Time { return now })
	report, err := m.SubscribedPeople(context.Background(), env.DB.Scoped(ws), w)
	require.NoError(t, err)
	return report
}

// subscribedByDay is the report's points by their date.
func subscribedByDay(report *analyticsreport.SubscribedPeople) map[string]analyticsreport.SubscribedPoint {
	days := map[string]analyticsreport.SubscribedPoint{}
	for _, p := range report.Series {
		days[p.Date] = p
	}
	return days
}

// Fixtures (Gotham, America/New_York), the window being 4-10 March 2026. Subscribed is the
// marketing.confirmed Event, unsubscribed the email.unsubscribed Event, each on the Report day of
// its occurred_at: 6 March one confirmed,
// 7 March one (23:59 EST), 8 March two confirmed and one unsubscribed (the midnight EST one, the
// 23:59 EDT one and the 01:00 one), 9 March one confirmed (backfilled: recorded on the 10th) and
// one unsubscribed (midnight EDT). A signup Event, an unsubscribe from February and a phone's SMS
// consent (sms.confirmed) on 6 March count nowhere.
//
// A Contact created with marketing allowed adds nothing of its own: Absence means subscribed
// (GLOSSARY "Unsubscribe"), a Contact has no marketing flag, and the marketing.confirmed Event is
// the one record of a person subscribing, so counting creations too would count them twice.
func TestSubscribedAndUnsubscribedAreCountedPerReportDay(t *testing.T) {
	env := testhelper.Setup(t)
	report := subscribedAt(t, env, fixtures.GothamID, analyticsreport.Window7Days, time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC))

	want := []analyticsreport.SubscribedPoint{
		{Date: "2026-03-04"},
		{Date: "2026-03-05"},
		{Date: "2026-03-06", Subscribed: 1, Net: 1},
		{Date: "2026-03-07", Subscribed: 1, Net: 1},
		{Date: "2026-03-08", Subscribed: 2, Unsubscribed: 1, Net: 1},
		{Date: "2026-03-09", Subscribed: 1, Unsubscribed: 1, Net: 0},
		{Date: "2026-03-10"},
	}
	assert.Equal(t, want, report.Series)
}

func TestAZoneChangeMovesSubscriptionsToOtherDays(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	inNewYork := subscribedByDay(subscribedAt(t, env, fixtures.GothamID, analyticsreport.Window7Days, now))

	require.NoError(t, env.DB.Workspace.UpdateOneID(fixtures.GothamID).SetTimezone("UTC").Exec(context.Background()))
	byDay := subscribedByDay(subscribedAt(t, env, fixtures.GothamID, analyticsreport.Window7Days, now))

	assert.Equal(t, 1, inNewYork["2026-03-07"].Subscribed, "23:59 EST on the 7th")
	assert.Equal(t, 0, byDay["2026-03-07"].Subscribed, "23:59 EST on the 7th is the 8th in UTC")
	assert.Equal(t, 2, byDay["2026-03-08"].Subscribed)
	assert.Equal(t, 2, byDay["2026-03-09"].Subscribed)
}

func TestAWorkspaceWithoutSubscriptionChangesHasAZeroFilledSeries(t *testing.T) {
	env := testhelper.Setup(t)
	report := subscribedAt(t, env, fixtures.GothamID, analyticsreport.Window30Days, time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))

	require.Len(t, report.Series, 30)
	for _, p := range report.Series {
		assert.Equal(t, analyticsreport.SubscribedPoint{Date: p.Date}, p)
	}
}

// The current number is read from Contact state with the dashboard's opt-out predicate, so it is
// the dashboard's Active figure whatever the series says.
func TestTheCurrentNumberIsTheContactsThatAreNotGloballyOptedOut(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.TyrellID)

	overview, err := analyticsreport.New().Overview(ctx, s, analyticsreport.Window7Days)
	require.NoError(t, err)
	report, err := analyticsreport.New().SubscribedPeople(ctx, s, analyticsreport.Window7Days)
	require.NoError(t, err)

	assert.Equal(t, overview.Contacts.Active, report.Current)
	assert.Equal(t, 3, overview.Contacts.Unsubscribed, "Tyrell holds three globally opted-out Contacts")
	assert.Positive(t, report.Current)
}
