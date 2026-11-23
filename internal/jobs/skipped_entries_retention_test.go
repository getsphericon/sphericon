package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// welcomeSkipped is the ids of the Welcome series' skipped entries, as the automations module lists
// them.
func welcomeSkipped(t *testing.T, env *testhelper.TestEnv) []int64 {
	t.Helper()
	page, err := testhelper.Module[*automations.Module](env).SkippedEntries(context.Background(), env.DB.Scoped(fixtures.AcmeID),
		pagination.Params{Page: 1, PageSize: 100}, automations.SkippedEntriesFilter{AutomationID: fixtures.AutomationWelcomeSeriesID})
	require.NoError(t, err)
	return lo.Map(page.Items, func(e *ent.AutomationSkippedEntry, _ int) int64 { return e.ID })
}

// pruneSkippedEntries works the daily prune of skipped entries with the retention window.
func pruneSkippedEntries(t *testing.T, env *testhelper.TestEnv, retention time.Duration) {
	t.Helper()
	w := jobs.NewPruneSkippedEntriesWorker(testhelper.Module[*automations.StepExecution](env), env.DB, retention)
	require.NoError(t, w.Work(context.Background(), job(jobs.PruneSkippedEntriesArgs{})))
}

// Skipped entries expire with the analytical Events (ADR 0019): the one past the window goes, the
// younger ones stay, and a zero retention keeps them all.
func TestPruneSkippedEntriesDeletesThosePastTheEventsWindow(t *testing.T) {
	env := testhelper.Setup(t)

	pruneSkippedEntries(t, env, eventsRetention)

	left := welcomeSkipped(t, env)
	assert.ElementsMatch(t, []int64{fixtures.SkippedEntryWelcomeLiamID, fixtures.SkippedEntryWelcomeEmmaID}, left)
}

func TestPruneSkippedEntriesKeepsThemWhenRetentionIsZero(t *testing.T) {
	env := testhelper.Setup(t)

	pruneSkippedEntries(t, env, 0)

	left := welcomeSkipped(t, env)
	assert.Contains(t, left, int64(fixtures.SkippedEntryExpiredID))
}
