package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/staff"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/trackinghostchange"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var erasureNow = time.Date(2026, time.October, 19, 12, 0, 0, 0, time.UTC)

// deleteWorkspace puts a Workspace into its deletion window, deleted the given time before now.
func deleteWorkspace(t *testing.T, env *testhelper.TestEnv, id int64, ago time.Duration) {
	t.Helper()
	env.DB.Workspace.UpdateOneID(id).SetDeletedAt(erasureNow.Add(-ago)).ExecX(t.Context())
}

// runErasure is one tick of the job at erasureNow, in batches of two so the loop is exercised.
func runErasure(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	w := jobs.NewEraseWorkspacesWorker(env.DB, env.Bus, func() time.Time { return erasureNow }, 2)
	require.NoError(t, w.Work(context.Background(), &river.Job[jobs.EraseWorkspacesArgs]{JobRow: &rivertype.JobRow{}}))
}

func rowsOf(t *testing.T, env *testhelper.TestEnv, ws int64) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, e := range ent.ScopedEntities() {
		ids, err := e.IDs(t.Context(), env.DB, ws)
		require.NoError(t, err)
		counts[e.Name] = len(ids)
	}
	return counts
}

func TestErasureCoversEveryWorkspaceOwnedEntity(t *testing.T) {
	assert.Len(t, ent.ScopedEntities(), jobs.ErasureStepCount())
}

func TestErasureLeavesAWorkspaceInsideItsWindow(t *testing.T) {
	env := testhelper.Setup(t)
	deleteWorkspace(t, env, fixtures.UmbrellaID, accounts.DeletionWindow-time.Hour)
	before := rowsOf(t, env, fixtures.UmbrellaID)

	runErasure(t, env)

	assert.Equal(t, before, rowsOf(t, env, fixtures.UmbrellaID))
	ws := env.DB.Workspace.GetX(t.Context(), fixtures.UmbrellaID)
	assert.Nil(t, ws.ErasedAt)
	assert.NotEmpty(t, ws.Name)
}

func TestErasureRemovesWhatAWorkspaceOwnedAndKeepsATombstone(t *testing.T) {
	env := testhelper.Setup(t)
	deleteWorkspace(t, env, fixtures.UmbrellaID, accounts.DeletionWindow+time.Hour)
	before := env.DB.Workspace.GetX(t.Context(), fixtures.UmbrellaID)
	acme := rowsOf(t, env, fixtures.AcmeID)
	require.NotZero(t, rowsOf(t, env, fixtures.UmbrellaID)["SendingDomain"])

	runErasure(t, env)

	for name, n := range rowsOf(t, env, fixtures.UmbrellaID) {
		assert.Zerof(t, n, "%s rows are erased", name)
	}
	assert.Equal(t, acme, rowsOf(t, env, fixtures.AcmeID), "another Workspace is untouched")
	ws := env.DB.Workspace.GetX(t.Context(), fixtures.UmbrellaID)
	assert.Equal(t, before.PublicID, ws.PublicID)
	assert.True(t, before.CreatedAt.Equal(ws.CreatedAt))
	assert.True(t, before.DeletedAt.Equal(*ws.DeletedAt))
	assert.True(t, erasureNow.Equal(*ws.ErasedAt))
	assert.Empty(t, ws.Name)
	assert.NotEqual(t, before.CollectKey, ws.CollectKey, "the collect key no longer opens anything")
}

func TestErasureReleasesTheTrackingHostsAndAdvancesTheFeed(t *testing.T) {
	env := testhelper.Setup(t)
	deleteWorkspace(t, env, fixtures.UmbrellaID, accounts.DeletionWindow+time.Hour)
	_, cursor := feedTail(t, env, "")

	runErasure(t, env)

	removed := env.DB.TrackingHostChange.Query().
		Where(trackinghostchange.WorkspaceID(fixtures.UmbrellaID), trackinghostchange.StateEQ(trackinghostchange.StateRemoved)).
		AllX(t.Context())
	var hosts []string
	for _, c := range removed {
		hosts = append(hosts, c.Domain)
	}
	assert.ElementsMatch(t, []string{"dns.umbrella.test", "serving.umbrella.test"}, hosts)

	entries, _ := feedTail(t, env, cursor)
	require.Len(t, entries, 1)
	tomb := entries[0]
	assert.Equal(t, int64(fixtures.UmbrellaID), tomb.Workspace.ID)
	assert.NotNil(t, tomb.Workspace.ErasedAt)
	assert.NotNil(t, tomb.Workspace.DeletedAt)
	assert.Empty(t, tomb.Workspace.Name)
	assert.Empty(t, tomb.Owners)
}

func TestASecondErasureRunChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	deleteWorkspace(t, env, fixtures.UmbrellaID, accounts.DeletionWindow+time.Hour)
	runErasure(t, env)
	_, cursor := feedTail(t, env, "")
	hostLog := env.DB.TrackingHostChange.Query().CountX(t.Context())
	first := env.DB.Workspace.GetX(t.Context(), fixtures.UmbrellaID)

	runErasure(t, env)

	entries, _ := feedTail(t, env, cursor)
	assert.Empty(t, entries, "the feed does not move")
	assert.Equal(t, hostLog, env.DB.TrackingHostChange.Query().CountX(t.Context()))
	assert.True(t, first.ErasedAt.Equal(*env.DB.Workspace.GetX(t.Context(), fixtures.UmbrellaID).ErasedAt))
}

// feedTail reads the Workspace feed after cursor and returns its entries and the next cursor.
func feedTail(t *testing.T, env *testhelper.TestEnv, cursor string) ([]staff.FeedEntry, string) {
	t.Helper()
	page, err := env.StaffMembers.WorkspaceFeed(t.Context(), cursor, 200)
	require.NoError(t, err)
	return page.Entries, page.NextCursor
}
