package workspacedeletion_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/hook"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/workspacedeletion"
)

var now = time.Date(2026, time.October, 19, 12, 0, 0, 0, time.UTC)

var actor = events.Actor{Kind: events.ActorUser}

func setup(t *testing.T) (*testhelper.TestEnv, *workspacedeletion.UseCase) {
	t.Helper()
	env := testhelper.Setup(t)
	clock := func() time.Time { return now }
	return env, workspacedeletion.New(env.DB, env.Bus, workspaces.New(env.DB, env.Bus, clock), clock)
}

// deletedAgo puts Initech into deletion, deleted ago before now.
func deletedAgo(t *testing.T, env *testhelper.TestEnv, ago time.Duration) {
	t.Helper()
	env.FeedDB.Workspace.UpdateOneID(fixtures.InitechID).SetDiscardedAt(now.Add(-ago)).ExecX(t.Context())
}

func initech(t *testing.T, env *testhelper.TestEnv) *ent.Workspace {
	t.Helper()
	return env.DB.Workspace.GetX(t.Context(), fixtures.InitechID)
}

func restore(t *testing.T, m *workspacedeletion.UseCase) error {
	t.Helper()
	_, err := m.Restore(t.Context(), workspacedeletion.RestoreInput{UserID: fixtures.InitechOwnerOlgaID, PublicID: fixtures.InitechPublicID, Actor: actor})
	return err
}

func TestRestoreInsideTheWindowBringsTheWorkspaceBack(t *testing.T) {
	env, m := setup(t)
	deletedAgo(t, env, workspaces.DeletionWindow-time.Hour)

	require.NoError(t, restore(t, m))

	assert.Nil(t, initech(t, env).DiscardedAt)
}

func TestRestoreAfterTheWindowIsRefused(t *testing.T) {
	env, m := setup(t)
	deletedAgo(t, env, workspaces.DeletionWindow+time.Hour)

	require.ErrorIs(t, restore(t, m), lifecycle.ErrInvalidTransition)

	assert.NotNil(t, initech(t, env).DiscardedAt, "a Workspace past its window stays deleted")
}

// An erased Workspace has no Memberships left, so the real Restore finds no Owner: the API
// answers 404, as for any Workspace the caller cannot see.
func TestRestoreAfterErasureFindsNoOwner(t *testing.T) {
	env, m := setup(t)
	deletedAgo(t, env, workspaces.DeletionWindow+time.Hour)
	_, err := m.EraseDue(t.Context())
	require.NoError(t, err)

	err = restore(t, m)

	require.ErrorIs(t, err, workspaces.ErrNotFound)
	assert.NotNil(t, initech(t, env).ErasedAt)
}

// A live Workspace is not due: the erase leaves its name and its collect key.
func TestEraseDueLeavesALiveWorkspace(t *testing.T) {
	env, m := setup(t)
	before := initech(t, env)

	_, err := m.EraseDue(t.Context())

	require.NoError(t, err)
	got := initech(t, env)
	assert.Nil(t, got.ErasedAt)
	assert.Equal(t, before.Name, got.Name, "a live Workspace keeps its name")
	assert.Equal(t, before.CollectKey, got.CollectKey)
}

// erasedJustBefore is an ent hook on the Workspace's compare-and-swap write: an erase that read
// the same state writes first, so the write that follows holds a stale read. It is registered on
// the restore's write only (restoreWrite), so the writes it causes do not run it again.
func erasedJustBefore(next ent.Mutator) ent.Mutator {
	return hook.WorkspaceFunc(func(ctx context.Context, m *ent.WorkspaceMutation) (ent.Value, error) {
		// test writes: a concurrent mover simulated inside a hook is the race under test
		if err := m.Client().Workspace.UpdateOneID(fixtures.InitechID).SetErasedAt(now).Exec(ctx); err != nil {
			return nil, err
		}
		return next.Mutate(ctx, m)
	})
}

// restoredJustBefore is an ent hook on the Workspace's compare-and-swap write: a restore that read
// the same state writes first, so the write that follows holds a stale read. It is registered on
// the tombstone's write only (tombstoneWrite).
func restoredJustBefore(next ent.Mutator) ent.Mutator {
	return hook.WorkspaceFunc(func(ctx context.Context, m *ent.WorkspaceMutation) (ent.Value, error) {
		// test writes: a concurrent mover simulated inside a hook is the race under test
		if err := m.Client().Workspace.UpdateOneID(fixtures.InitechID).ClearDiscardedAt().Exec(ctx); err != nil {
			return nil, err
		}
		return next.Mutate(ctx, m)
	})
}

// restoreWrite is the restore's compare-and-swap: the conditional update that clears discarded_at.
var restoreWrite = hook.And(hook.HasOp(ent.OpUpdate), hook.HasClearedFields(workspace.FieldDiscardedAt))

// tombstoneWrite is the erase's compare-and-swap: the conditional update that sets erased_at.
var tombstoneWrite = hook.And(hook.HasOp(ent.OpUpdate), hook.HasFields(workspace.FieldErasedAt))

// A restore that read the Workspace as pending deletion loses to an erase that wrote first: its
// compare-and-swap finds the row moved, and the Workspace is not brought back.
func TestARestoreThatLosesToAnEraseIsRefused(t *testing.T) {
	env, m := setup(t)
	deletedAgo(t, env, time.Hour)
	env.Bus.Use(hook.If(erasedJustBefore, restoreWrite))

	err := restore(t, m)

	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
	assert.NotNil(t, initech(t, env).DiscardedAt, "the Workspace stays deleted")
}

// An erase that read the Workspace as pending deletion loses to a restore that wrote first: the
// stale move is finished, not an error, and no tombstone is written.
func TestAnEraseThatLosesToARestoreIsFinishedWithoutATombstone(t *testing.T) {
	env, m := setup(t)
	deletedAgo(t, env, workspaces.DeletionWindow+time.Hour)
	name := initech(t, env).Name
	env.Bus.Use(hook.If(restoredJustBefore, tombstoneWrite))

	erased, err := m.EraseDue(t.Context())

	require.NoError(t, err)
	assert.Zero(t, erased)
	got := initech(t, env)
	assert.Nil(t, got.ErasedAt)
	assert.Equal(t, name, got.Name)
}

// The deleted Workspaces of an Owner say which have no other Owner to restore them: Bluth is Dan's
// alone, Initech has two Owners.
func TestDeletedSaysWhichWorkspacesHaveOneOwner(t *testing.T) {
	env, m := setup(t)
	deletedAgo(t, env, time.Hour)

	dans, err := m.Deleted(t.Context(), fixtures.DeletedWorkspaceOwnerDanID)
	require.NoError(t, err)
	olgas, err := m.Deleted(t.Context(), fixtures.InitechOwnerOlgaID)
	require.NoError(t, err)

	require.Len(t, dans, 1)
	assert.Equal(t, fixtures.BluthName, dans[0].Workspace.Name)
	assert.True(t, dans[0].OnlyOwner)
	require.Len(t, olgas, 1)
	assert.Equal(t, int64(fixtures.InitechID), olgas[0].Workspace.ID)
	assert.False(t, olgas[0].OnlyOwner)
}

// A deletion answers the deleted Workspace with whether its Owner is its only one.
func TestDeleteAnswersWhetherTheOwnerIsTheOnlyOne(t *testing.T) {
	env, m := setup(t)

	aviato, err := m.Delete(t.Context(), env.DB.Scoped(fixtures.AviatoID), actor, fixtures.AviatoName)
	require.NoError(t, err)
	initechDeleted, err := m.Delete(t.Context(), env.DB.Scoped(fixtures.InitechID), actor, initech(t, env).Name)
	require.NoError(t, err)

	assert.NotNil(t, aviato.Workspace.DiscardedAt)
	assert.True(t, aviato.OnlyOwner, "Aviato is Dora's alone")
	assert.False(t, initechDeleted.OnlyOwner, "Initech has two Owners")
}
