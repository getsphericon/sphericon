package workspacedeletion_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/staff"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/hook"
	"github.com/getsphericon/sphericon/ent/trackinghostchange"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/workspacedeletion"
)

// deleteUmbrella puts Umbrella into its deletion window, deleted ago before now.
func deleteUmbrella(t *testing.T, env *testhelper.TestEnv, ago time.Duration) {
	t.Helper()
	env.DB.Workspace.UpdateOneID(fixtures.UmbrellaID).SetDeletedAt(now.Add(-ago)).ExecX(t.Context())
}

// runErase is one erase at now, in batches of two so the batch loop is exercised.
func runErase(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	m := workspacedeletion.WithBatch(workspacedeletion.New(env.DB, env.Bus), 2)
	_, err := m.EraseDue(context.Background(), now)
	require.NoError(t, err)
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

func TestEraseCoversEveryWorkspaceOwnedEntity(t *testing.T) {
	assert.Len(t, ent.ScopedEntities(), workspacedeletion.EraseStepCount())
}

func TestEraseDueLeavesAWorkspaceInsideItsWindow(t *testing.T) {
	env := testhelper.Setup(t)
	deleteUmbrella(t, env, workspacedeletion.DeletionWindow-time.Hour)
	before := rowsOf(t, env, fixtures.UmbrellaID)

	runErase(t, env)

	assert.Equal(t, before, rowsOf(t, env, fixtures.UmbrellaID))
	ws := env.DB.Workspace.GetX(t.Context(), fixtures.UmbrellaID)
	assert.Nil(t, ws.ErasedAt)
	assert.NotEmpty(t, ws.Name)
}

func TestEraseDueRemovesWhatAWorkspaceOwnedAndKeepsATombstone(t *testing.T) {
	env := testhelper.Setup(t)
	deleteUmbrella(t, env, workspacedeletion.DeletionWindow+time.Hour)
	before := env.DB.Workspace.GetX(t.Context(), fixtures.UmbrellaID)
	acme := rowsOf(t, env, fixtures.AcmeID)
	require.NotZero(t, rowsOf(t, env, fixtures.UmbrellaID)["SendingDomain"])

	runErase(t, env)

	for name, n := range rowsOf(t, env, fixtures.UmbrellaID) {
		assert.Zerof(t, n, "%s rows are erased", name)
	}
	assert.Equal(t, acme, rowsOf(t, env, fixtures.AcmeID), "another Workspace is untouched")
	ws := env.DB.Workspace.GetX(t.Context(), fixtures.UmbrellaID)
	assert.Equal(t, before.PublicID, ws.PublicID)
	assert.True(t, before.CreatedAt.Equal(ws.CreatedAt))
	assert.True(t, before.DeletedAt.Equal(*ws.DeletedAt))
	assert.True(t, now.Equal(*ws.ErasedAt))
	assert.Empty(t, ws.Name)
	assert.NotEqual(t, before.CollectKey, ws.CollectKey, "the collect key no longer opens anything")
}

func TestEraseDueReleasesTheTrackingHostsAndAdvancesTheFeed(t *testing.T) {
	env := testhelper.Setup(t)
	deleteUmbrella(t, env, workspacedeletion.DeletionWindow+time.Hour)
	_, cursor := feedTail(t, env, "")

	runErase(t, env)

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

// Deleting a row can write another: here deleting a Contact writes an Event, after the
// Events step has run. The erasure settles, so nothing is left behind the tombstone.
func TestEraseDueRemovesRowsWrittenWhileItDeletes(t *testing.T) {
	env := testhelper.Setup(t)
	deleteUmbrella(t, env, workspacedeletion.DeletionWindow+time.Hour)
	env.DB.Contact.Use(func(next ent.Mutator) ent.Mutator {
		return hook.ContactFunc(func(ctx context.Context, m *ent.ContactMutation) (ent.Value, error) {
			v, err := next.Mutate(ctx, m)
			if err != nil || !m.Op().Is(ent.OpDelete|ent.OpDeleteOne) {
				return v, err
			}
			return v, m.Client().Event.Create().SetWorkspaceID(fixtures.UmbrellaID).SetAction("written.during.erasure").Exec(ctx)
		})
	})

	runErase(t, env)

	for name, n := range rowsOf(t, env, fixtures.UmbrellaID) {
		assert.Zerof(t, n, "%s rows are erased", name)
	}
	assert.NotNil(t, env.DB.Workspace.GetX(t.Context(), fixtures.UmbrellaID).ErasedAt)
}

func TestASecondEraseRunChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	deleteUmbrella(t, env, workspacedeletion.DeletionWindow+time.Hour)
	runErase(t, env)
	_, cursor := feedTail(t, env, "")
	hostLog := env.DB.TrackingHostChange.Query().CountX(t.Context())
	first := env.DB.Workspace.GetX(t.Context(), fixtures.UmbrellaID)

	runErase(t, env)

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
