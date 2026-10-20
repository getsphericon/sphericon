package workspacedeletion_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
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

// rowsOfAll is the rows of every entity across every Workspace of the fixtures.
func rowsOfAll(t *testing.T, env *testhelper.TestEnv) map[string]int {
	t.Helper()
	ids := env.DB.Workspace.Query().IDsX(t.Context())
	counts := map[string]int{}
	for _, ws := range ids {
		for name, n := range rowsOf(t, env, ws) {
			counts[name] += n
		}
	}
	return counts
}

// Every Workspace of the fixtures is erased, so an entity with no step leaves its rows behind.
// The fixtures hold rows of every scoped entity, which the first check requires.
func TestEraseCoversEveryWorkspaceOwnedEntity(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.Update().SetDeletedAt(now.Add(-workspacedeletion.DeletionWindow - time.Hour)).ExecX(t.Context())
	before := rowsOfAll(t, env)
	for _, e := range ent.ScopedEntities() {
		assert.Positivef(t, before[e.Name], "the fixtures hold a %s row", e.Name)
	}

	runErase(t, env)

	for name, n := range rowsOfAll(t, env) {
		assert.Zerof(t, n, "%s rows are erased", name)
	}
}

// writeEventAfterContactDelete is an ent hook that, after a Contact is deleted, writes an Event of
// Umbrella: a row written while the erase runs.
func writeEventAfterContactDelete(next ent.Mutator) ent.Mutator {
	return hook.ContactFunc(func(ctx context.Context, m *ent.ContactMutation) (ent.Value, error) {
		v, err := next.Mutate(ctx, m)
		if err != nil || !m.Op().Is(ent.OpDelete|ent.OpDeleteOne) {
			return v, err
		}
		return v, m.Client().Event.Create().SetWorkspaceID(fixtures.UmbrellaID).SetAction("written.during.erasure").Exec(ctx)
	})
}

func TestEraseDueLeavesAWorkspaceInsideItsWindow(t *testing.T) {
	env := testhelper.Setup(t)
	deleteUmbrella(t, env, workspacedeletion.DeletionWindow-time.Hour)
	before := rowsOf(t, env, fixtures.UmbrellaID)

	runErase(t, env)

	after := rowsOf(t, env, fixtures.UmbrellaID)
	assert.Equal(t, before, after)
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
	acmeAfter := rowsOf(t, env, fixtures.AcmeID)
	assert.Equal(t, acme, acmeAfter, "another Workspace is untouched")
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
	hosts := lo.Map(removed, func(c *ent.TrackingHostChange, _ int) string { return c.Domain })
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
	env.DB.Contact.Use(writeEventAfterContactDelete)

	runErase(t, env)

	for name, n := range rowsOf(t, env, fixtures.UmbrellaID) {
		assert.Zerof(t, n, "%s rows are erased", name)
	}
	ws := env.DB.Workspace.GetX(t.Context(), fixtures.UmbrellaID)
	assert.NotNil(t, ws.ErasedAt)
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
	hostLogAfter := env.DB.TrackingHostChange.Query().CountX(t.Context())
	second := env.DB.Workspace.GetX(t.Context(), fixtures.UmbrellaID)
	assert.Empty(t, entries, "the feed does not move")
	assert.Equal(t, hostLog, hostLogAfter)
	assert.True(t, first.ErasedAt.Equal(*second.ErasedAt))
}

// feedTail reads the Workspace feed after cursor and returns its entries and the next cursor.
func feedTail(t *testing.T, env *testhelper.TestEnv, cursor string) ([]staff.FeedEntry, string) {
	t.Helper()
	page, err := env.StaffMembers.WorkspaceFeed(t.Context(), cursor, 200)
	require.NoError(t, err)
	return page.Entries, page.NextCursor
}
