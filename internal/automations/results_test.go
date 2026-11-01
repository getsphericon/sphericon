package automations_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/orders"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Fixtures (Gotham, America/New_York, Reporting currency USD): the Enrollments of Gotham results
// begin and end at fixed moments around 8 March 2026 (automation_runs.yml), and two Orders credited
// to it move on 5, 12, 13 and 15 March (order_movements.yml). Read on 17 March over 7 days, the
// current period is 11-17 March and the previous one 4-10 March.
var gothamToday = time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)

func resultsAt(t *testing.T, env *testhelper.TestEnv, id int64, p automations.ReportPeriod, now time.Time) automations.Results {
	t.Helper()
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule()).WithClock(func() time.Time { return now })
	report, err := m.ReportOf(context.Background(), env.DB.Scoped(fixtures.GothamID), id, p)
	require.NoError(t, err)
	return report.Results
}

func usd(c orders.Credited) orders.Revenue {
	return orders.Revenue{Credited: c, Currency: new("USD")}
}

func TestResultsCountWhatHappenedInEachPeriodByReportDay(t *testing.T) {
	env := testhelper.Setup(t)

	results := resultsAt(t, env, fixtures.AutomationGothamResultsID, automations.ReportPeriod7Days, gothamToday)

	assert.Equal(t, automations.ResultCounts{Entered: 4, Completed: 1, GoalReached: 1, Exited: 1, Failed: 1}, results.Current.ResultCounts,
		"entered by when each began, an outcome by when it ended: the failure of an Enrollment that began on 9 March counts now")
	assert.Equal(t, automations.ResultCounts{Entered: 3, Completed: 1, GoalReached: 1, Exited: 1}, results.Previous.ResultCounts,
		"23:59 EDT on 10 March is the previous period; a goal reached on 4 March counts though its Enrollment began before")
	assert.Equal(t, "2026-03-11", results.Current.From)
	assert.Equal(t, "2026-03-17", results.Current.To)
	assert.Equal(t, "2026-03-04", results.Previous.From)
	assert.Equal(t, "2026-03-10", results.Previous.To)
}

func TestResultsHaveADayForEachReportDayOfThePeriod(t *testing.T) {
	env := testhelper.Setup(t)

	results := resultsAt(t, env, fixtures.AutomationGothamResultsID, automations.ReportPeriod7Days, gothamToday)

	assert.Equal(t, []automations.ResultsPoint{
		{Date: "2026-03-11", Entered: 1, Failed: 1},
		{Date: "2026-03-12", Entered: 1, Completed: 1, GoalReached: 1},
		{Date: "2026-03-13", Entered: 1},
		{Date: "2026-03-14", Entered: 1, Exited: 1},
		{Date: "2026-03-15"},
		{Date: "2026-03-16"},
		{Date: "2026-03-17"},
	}, results.Series)
}

func TestTheAverageDurationIsOverTheEnrollmentsThatEndedInThePeriod(t *testing.T) {
	env := testhelper.Setup(t)

	results := resultsAt(t, env, fixtures.AutomationGothamResultsID, automations.ReportPeriod7Days, gothamToday)

	assert.Equal(t, new(85050*time.Second), results.Current.AverageDuration,
		"24 h, 6 h, 24 h and 40.5 h; the active Enrollment has no duration yet")
	assert.Equal(t, new(57600*time.Second), results.Previous.AverageDuration, "23 h 59 min, 1 min and 24 h")
}

func TestAPeriodInWhichNoEnrollmentEndedHasNoAverageDuration(t *testing.T) {
	env := testhelper.Setup(t)

	results := resultsAt(t, env, fixtures.AutomationGothamResultsID, automations.ReportPeriod7Days, time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC))

	assert.Equal(t, automations.ResultCounts{}, results.Current.ResultCounts)
	assert.Nil(t, results.Current.AverageDuration)
	assert.Len(t, results.Series, 7)
}

func TestTheRevenueOfAPeriodIsTheMovementsOfTheCreditedOrdersInIt(t *testing.T) {
	env := testhelper.Setup(t)

	results := resultsAt(t, env, fixtures.AutomationGothamResultsID, automations.ReportPeriod7Days, gothamToday)

	assert.Equal(t, usd(orders.Credited{Orders: 1, Amount: 3500}), results.Current.Revenue,
		"50.00 completed on 12 March less 10.00 on 13 March and the 5.00 refund of the older Order on 15 March; the EUR Order counts nowhere")
	assert.Equal(t, usd(orders.Credited{Orders: 1, Amount: 2500}), results.Previous.Revenue)
}

func TestALongerPeriodTakesInBothPeriodsOfAShorterOne(t *testing.T) {
	env := testhelper.Setup(t)

	results := resultsAt(t, env, fixtures.AutomationGothamResultsID, automations.ReportPeriod14Days, gothamToday)

	assert.Equal(t, automations.ResultCounts{Entered: 7, Completed: 2, GoalReached: 2, Exited: 2, Failed: 1}, results.Current.ResultCounts)
	assert.Equal(t, automations.ResultCounts{Entered: 2, Completed: 1}, results.Previous.ResultCounts,
		"the Enrollment of 20 February-2 March, and the one that began on 3 March")
	assert.Len(t, results.Series, 14)
}

func TestAZoneChangeMovesResultsToOtherDays(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, env.DB.Workspace.UpdateOneID(fixtures.GothamID).SetTimezone("UTC").Exec(context.Background()))

	results := resultsAt(t, env, fixtures.AutomationGothamResultsID, automations.ReportPeriod7Days, gothamToday)

	assert.Equal(t, automations.ResultCounts{Entered: 4, Completed: 2, GoalReached: 1, Exited: 1, Failed: 1}, results.Current.ResultCounts,
		"in UTC the completion at 03:59 on 11 March belongs to the current period")
}

func TestResultsOfAnotherWorkspacesAutomationAreNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())

	_, err := m.ReportOf(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationGothamResultsID, automations.ReportPeriod7Days)

	require.ErrorIs(t, err, automations.ErrNotFound)
}
