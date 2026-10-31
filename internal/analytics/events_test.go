package analytics_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/analytics"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func eventActions(t *testing.T, env *testhelper.TestEnv, ws int64, w analytics.Window) []analytics.EventAction {
	t.Helper()
	rows, err := analytics.New().Events(context.Background(), env.DB.Scoped(ws), w)
	require.NoError(t, err)
	return rows
}

func actionRow(t *testing.T, rows []analytics.EventAction, action string) analytics.EventAction {
	t.Helper()
	row, ok := lo.Find(rows, func(r analytics.EventAction) bool { return r.Action == action })
	require.True(t, ok, action)
	return row
}

func actionNames(rows []analytics.EventAction) []string {
	return lo.Map(rows, func(r analytics.EventAction, _ int) string { return r.Action })
}

// Fixtures (events.yml, "Events report"): report_view has four Events of two Contacts and an
// anonymous visitor, so the anonymous one counts as an occurrence and not as a Contact.
func TestEventsReportCountsOccurrencesAndDistinctContactsPerAction(t *testing.T) {
	env := testhelper.Setup(t)
	rows := eventActions(t, env, fixtures.AcmeID, analytics.Window7Days)

	view := actionRow(t, rows, "report_view")
	assert.Equal(t, 4, view.Count)
	assert.Equal(t, 2, view.Contacts)
}

func TestEventsReportLastOccurrenceIsTheNewestOccurredAt(t *testing.T) {
	env := testhelper.Setup(t)
	rows := eventActions(t, env, fixtures.AcmeID, analytics.Window7Days)

	newest := env.DB.Event.GetX(context.Background(), 5602)
	view := actionRow(t, rows, "report_view")
	assert.True(t, newest.OccurredAt.Equal(view.LastAt), "last %s, newest %s", view.LastAt, newest.OccurredAt)
}

func TestEventsReportFollowsWhenItHappenedNotWhenItWasRecorded(t *testing.T) {
	env := testhelper.Setup(t)
	week := actionNames(eventActions(t, env, fixtures.AcmeID, analytics.Window7Days))
	month := actionNames(eventActions(t, env, fixtures.AcmeID, analytics.Window30Days))
	quarter := actionNames(eventActions(t, env, fixtures.AcmeID, analytics.Window90Days))

	assert.NotContains(t, week, "report_backfill")
	assert.Contains(t, month, "report_backfill")
	assert.NotContains(t, month, "report_old")
	assert.Contains(t, quarter, "report_old")
}

func TestEventsReportListsReservedActionsAndLeavesMachineOpensOut(t *testing.T) {
	env := testhelper.Setup(t)
	rows := eventActions(t, env, fixtures.AcmeID, analytics.Window7Days)

	opened := actionRow(t, rows, "email.opened")
	assert.Equal(t, 2, opened.Count)
	assert.Equal(t, 1, opened.Contacts)
	sent := actionRow(t, rows, "email.sent")
	assert.Positive(t, sent.Count)
}

func TestEventsReportIsOrderedByCountThenAction(t *testing.T) {
	env := testhelper.Setup(t)
	rows := eventActions(t, env, fixtures.AcmeID, analytics.Window90Days)

	counts := lo.Map(rows, func(r analytics.EventAction, _ int) int { return r.Count })
	assert.IsNonIncreasing(t, counts)
}

func TestEventsReportStaysInItsWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	rows := eventActions(t, env, fixtures.AcmeID, analytics.Window90Days)

	assert.NotContains(t, actionNames(rows), "globex_only")
}
