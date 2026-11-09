package workspacesummary_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/hook"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/workspacedeletion"
	"github.com/getsphericon/sphericon/internal/usecase/workspacesummary"
)

var restoredAt = time.Date(2026, time.October, 19, 12, 0, 0, 0, time.UTC)

func clock() time.Time { return restoredAt }

// restoreEnv is the test's Instance on the clock restoredAt.
func restoreEnv(t *testing.T) *testhelper.TestEnv {
	t.Helper()
	return testhelper.Setup(t, testhelper.WithClock(clock))
}

// deletedAgo puts Initech into deletion, deleted ago before restoredAt.
func deletedAgo(t *testing.T, env *testhelper.TestEnv, ago time.Duration) {
	t.Helper()
	env.FeedDB.Workspace.UpdateOneID(fixtures.InitechID).SetDiscardedAt(restoredAt.Add(-ago)).ExecX(t.Context())
}

func initech(t *testing.T, env *testhelper.TestEnv) *ent.Workspace {
	t.Helper()
	return env.DB.Workspace.GetX(t.Context(), fixtures.InitechID)
}

// restore is Olga, an Owner of Initech, restoring it.
func restore(t *testing.T, env *testhelper.TestEnv) (workspacesummary.View, error) {
	t.Helper()
	return env.WorkspaceSummary.Restore(t.Context(), workspacesummary.RestoreInput{
		UserID: fixtures.InitechOwnerOlgaID, PublicID: fixtures.InitechPublicID, Actor: events.Actor{Kind: events.ActorUser},
	})
}

func TestRestoreInsideTheWindowBringsTheWorkspaceBack(t *testing.T) {
	env := restoreEnv(t)
	deletedAgo(t, env, workspaces.DeletionWindow-time.Hour)

	v, err := restore(t, env)

	require.NoError(t, err)
	assert.Nil(t, initech(t, env).DiscardedAt)
	assert.Equal(t, int64(fixtures.InitechID), v.Summary.Workspace.ID, "the View is of the restored Workspace")
	assert.Nil(t, v.Summary.Workspace.DiscardedAt)
	assert.Equal(t, membership.RoleOwner, v.Membership.Role, "as its Owner reads it")
}

func TestRestoreAfterTheWindowIsRefused(t *testing.T) {
	env := restoreEnv(t)
	deletedAgo(t, env, workspaces.DeletionWindow+time.Hour)

	_, err := restore(t, env)

	require.ErrorIs(t, err, workspacesummary.ErrNotRestorable)
	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
	assert.NotNil(t, initech(t, env).DiscardedAt, "a Workspace past its window stays deleted")
}

// An erased Workspace has no Memberships left, so the real Restore finds no Owner: the API
// answers 404, as for any Workspace the caller cannot see.
func TestRestoreAfterErasureFindsNoOwner(t *testing.T) {
	env := restoreEnv(t)
	deletedAgo(t, env, workspaces.DeletionWindow+time.Hour)
	_, err := workspacedeletion.New(env.DB, env.Bus, workspaces.New(env.DB, env.Bus, clock), clock).EraseDue(t.Context())
	require.NoError(t, err)

	_, err = restore(t, env)

	require.ErrorIs(t, err, workspaces.ErrNotFound)
	assert.NotNil(t, initech(t, env).ErasedAt)
}

// erasedJustBefore is an ent hook on the Workspace's compare-and-swap write: an erase that read
// the same state writes first, so the write that follows holds a stale read. It is registered on
// the restore's write only (restoreWrite), so the writes it causes do not run it again.
func erasedJustBefore(next ent.Mutator) ent.Mutator {
	return hook.WorkspaceFunc(func(ctx context.Context, m *ent.WorkspaceMutation) (ent.Value, error) {
		// test writes: a concurrent mover simulated inside a hook is the race under test
		if err := m.Client().Workspace.UpdateOneID(fixtures.InitechID).SetErasedAt(restoredAt).Exec(ctx); err != nil {
			return nil, err
		}
		return next.Mutate(ctx, m)
	})
}

// restoreWrite is the restore's compare-and-swap: the conditional update that clears discarded_at.
var restoreWrite = hook.And(hook.HasOp(ent.OpUpdate), hook.HasClearedFields(workspace.FieldDiscardedAt))

// A restore that read the Workspace as pending deletion loses to an erase that wrote first: its
// compare-and-swap finds the row moved, and the Workspace is not brought back.
func TestARestoreThatLosesToAnEraseIsRefused(t *testing.T) {
	env := restoreEnv(t)
	deletedAgo(t, env, time.Hour)
	env.Bus.Use(hook.If(erasedJustBefore, restoreWrite))

	_, err := restore(t, env)

	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
	assert.NotNil(t, initech(t, env).DiscardedAt, "the Workspace stays deleted")
}
