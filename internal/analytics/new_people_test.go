package analytics_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/analytics"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func newPeopleAt(t *testing.T, env *testhelper.TestEnv, ws int64, w analytics.Window, now time.Time) *analytics.NewPeople {
	t.Helper()
	m := analytics.New().WithClock(func() time.Time { return now })
	report, err := m.NewPeople(context.Background(), env.DB.Scoped(ws), w)
	require.NoError(t, err)
	return report
}

func byDay(report *analytics.NewPeople) map[string]analytics.NewPeoplePoint {
	days := map[string]analytics.NewPeoplePoint{}
	for _, p := range report.Series {
		days[p.Date] = p
	}
	return days
}

// Fixtures (Gotham, America/New_York): sign-ups at 2026-03-08 04:59Z (23:59 EST on 7 March, manual),
// 05:00Z (midnight EST, api), 12:00Z (8 March, api), and 2026-03-09 03:59Z (23:59 EDT on 8 March,
// tracker) and 04:00Z (9 March, no source: it predates the field).
func TestNewPeopleAreCountedPerReportDayAndSplitBySource(t *testing.T) {
	env := testhelper.Setup(t)
	report := newPeopleAt(t, env, fixtures.GothamID, analytics.Window7Days, time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC))

	require.Len(t, report.Series, 7, "one point per day, the 23-hour day included once")
	days := byDay(report)
	assert.Equal(t, analytics.NewPeoplePoint{Date: "2026-03-07", Manual: 1}, days["2026-03-07"])
	assert.Equal(t, analytics.NewPeoplePoint{Date: "2026-03-08", API: 2, Tracker: 1}, days["2026-03-08"])
	assert.Equal(t, analytics.NewPeoplePoint{Date: "2026-03-09", Unknown: 1}, days["2026-03-09"])
	assert.Equal(t, analytics.NewPeoplePoint{Date: "2026-03-10"}, days["2026-03-10"], "an empty day is all zeros")
	assert.Equal(t, 5, report.Total)
}

func TestNewPeopleOfAWindowLeavesOutContactsBeforeIt(t *testing.T) {
	env := testhelper.Setup(t)
	report := newPeopleAt(t, env, fixtures.GothamID, analytics.Window7Days, time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC))

	assert.Equal(t, 4, report.Total, "the last day of the window ends at midnight EDT; the 9 March sign-up and the 2 January ones are outside it")
}

func TestAZoneChangeMovesNewPeopleToOtherDays(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	require.NoError(t, env.DB.Workspace.UpdateOneID(fixtures.GothamID).SetTimezone("UTC").Exec(context.Background()))

	days := byDay(newPeopleAt(t, env, fixtures.GothamID, analytics.Window7Days, now))

	assert.Equal(t, analytics.NewPeoplePoint{Date: "2026-03-07"}, days["2026-03-07"])
	assert.Equal(t, analytics.NewPeoplePoint{Date: "2026-03-08", Manual: 1, API: 2}, days["2026-03-08"])
	assert.Equal(t, analytics.NewPeoplePoint{Date: "2026-03-09", Tracker: 1, Unknown: 1}, days["2026-03-09"])
}

func TestAWorkspaceWithoutNewPeopleHasAZeroFilledSeries(t *testing.T) {
	env := testhelper.Setup(t)
	report := newPeopleAt(t, env, fixtures.GlobexID, analytics.Window30Days, time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC))

	assert.Zero(t, report.Total)
	assert.Len(t, report.Series, 30)
}
