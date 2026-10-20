package workspacedeletion_test

import (
	"context"
	"testing"
	"time"

	"github.com/qmuntal/stateless"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/workspacedeletion"
)

var now = time.Date(2026, time.October, 19, 12, 0, 0, 0, time.UTC)

var actor = events.Actor{Kind: events.ActorUser}

func setup(t *testing.T) (*testhelper.TestEnv, *workspacedeletion.Module) {
	t.Helper()
	env := testhelper.Setup(t)
	return env, workspacedeletion.New(env.DB, env.Bus)
}

// deletedAgo puts Initech into deletion, deleted ago before now.
func deletedAgo(t *testing.T, env *testhelper.TestEnv, ago time.Duration) {
	t.Helper()
	env.DB.Workspace.UpdateOneID(fixtures.InitechID).SetDeletedAt(now.Add(-ago)).ExecX(t.Context())
}

func initech(t *testing.T, env *testhelper.TestEnv) *ent.Workspace {
	t.Helper()
	return env.DB.Workspace.GetX(t.Context(), fixtures.InitechID)
}

func erase(t *testing.T, env *testhelper.TestEnv, m *workspacedeletion.Module) error {
	t.Helper()
	return env.Bus.WithinTx(t.Context(), func(tx *ent.Client, _ events.Publisher) error {
		return workspacedeletion.Tombstone(t.Context(), tx, fixtures.InitechID, now)
	})
}

func restore(t *testing.T, m *workspacedeletion.Module) error {
	t.Helper()
	_, err := m.Restore(t.Context(), fixtures.InitechOwnerOlgaID, fixtures.InitechPublicID, actor, now)
	return err
}

func TestRestoreInsideTheWindowBringsTheWorkspaceBack(t *testing.T) {
	env, m := setup(t)
	deletedAgo(t, env, workspacedeletion.DeletionWindow-time.Hour)

	require.NoError(t, restore(t, m))

	assert.Nil(t, initech(t, env).DeletedAt)
}

func TestRestoreAfterTheWindowIsRefused(t *testing.T) {
	env, m := setup(t)
	deletedAgo(t, env, workspacedeletion.DeletionWindow+time.Hour)

	require.ErrorIs(t, restore(t, m), lifecycle.ErrInvalidTransition)

	assert.NotNil(t, initech(t, env).DeletedAt, "a Workspace past its window stays deleted")
}

// An erased Workspace has no Memberships left, so the real Restore finds no Owner: the API
// answers 404, as for any Workspace the caller cannot see.
func TestRestoreAfterErasureFindsNoOwner(t *testing.T) {
	env, m := setup(t)
	deletedAgo(t, env, workspacedeletion.DeletionWindow+time.Hour)
	_, err := m.EraseDue(t.Context(), now)
	require.NoError(t, err)

	err = restore(t, m)

	assert.True(t, ent.IsNotFound(err), "got %v", err)
	assert.NotNil(t, initech(t, env).ErasedAt)
}

// The machine itself refuses a restore of an erased row, whatever the caller could see.
func TestTheMachineRefusesRestoringAnErasedWorkspace(t *testing.T) {
	env, m := setup(t)
	deletedAgo(t, env, workspacedeletion.DeletionWindow+time.Hour)
	require.NoError(t, erase(t, env, m))
	machine := workspacedeletion.Machine(workspacedeletion.Storage(env.DB, fixtures.InitechID, now))

	err := machine.FireCtx(t.Context(), workspacedeletion.TriggerRestore)

	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
	got := initech(t, env)
	assert.NotNil(t, got.DeletedAt)
	assert.NotNil(t, got.ErasedAt)
}

func TestEraseOfAWorkspaceThatIsNotDeletedIsRefused(t *testing.T) {
	env, m := setup(t)
	before := initech(t, env)

	require.ErrorIs(t, erase(t, env, m), lifecycle.ErrInvalidTransition)

	got := initech(t, env)
	assert.Nil(t, got.ErasedAt)
	assert.Equal(t, before.Name, got.Name, "a live Workspace keeps its name")
	assert.Equal(t, before.CollectKey, got.CollectKey)
}

func TestEraseLeavesATombstone(t *testing.T) {
	env, m := setup(t)
	before := initech(t, env)
	deletedAgo(t, env, workspacedeletion.DeletionWindow+time.Hour)

	require.NoError(t, erase(t, env, m))

	got := initech(t, env)
	require.NotNil(t, got.ErasedAt)
	assert.True(t, now.Equal(*got.ErasedAt))
	assert.Empty(t, got.Name)
	assert.Empty(t, got.PostalAddress)
	assert.NotEqual(t, before.CollectKey, got.CollectKey)
}

func TestEraseTwiceIsRefused(t *testing.T) {
	env, m := setup(t)
	deletedAgo(t, env, workspacedeletion.DeletionWindow+time.Hour)
	require.NoError(t, erase(t, env, m))

	require.ErrorIs(t, erase(t, env, m), lifecycle.ErrInvalidTransition)
}

func TestDeletingTwiceKeepsTheFirstDeletionTime(t *testing.T) {
	env, m := setup(t)
	s := env.DB.Scoped(fixtures.InitechID)
	name := initech(t, env).Name

	first, err := m.Delete(t.Context(), s, actor, name, now)
	require.NoError(t, err)
	second, err := m.Delete(t.Context(), s, actor, name, now.Add(24*time.Hour))
	require.NoError(t, err)

	require.NotNil(t, first.DeletedAt)
	require.NotNil(t, second.DeletedAt)
	assert.True(t, first.DeletedAt.Equal(*second.DeletedAt), "the window does not restart")
}

func TestDeleteRefusesAWrongName(t *testing.T) {
	env, m := setup(t)

	_, err := m.Delete(t.Context(), env.DB.Scoped(fixtures.InitechID), actor, "not the name", now)

	require.ErrorIs(t, err, workspacedeletion.ErrNameMismatch)
	assert.Nil(t, initech(t, env).DeletedAt)
}

// stale is a mover that read the Workspace as pending deletion before anyone wrote.
func stale(env *testhelper.TestEnv) *stateless.StateMachine {
	st := workspacedeletion.Storage(env.DB, fixtures.InitechID, now)
	st.Read = func(context.Context) (workspacedeletion.State, error) {
		return workspacedeletion.StatePendingDeletion, nil
	}
	return workspacedeletion.Machine(st)
}

// A restore and an erase that both read the Workspace as pending deletion race on one row:
// the first write wins and the second one, whose read is stale, loses the compare-and-swap.
func TestARestoreRacingAnEraseHasOneWinner(t *testing.T) {
	cases := []struct {
		name          string
		first, second workspacedeletion.Trigger
		want          workspacedeletion.State
	}{
		{"restore first", workspacedeletion.TriggerRestore, workspacedeletion.TriggerErase, workspacedeletion.StateActive},
		{"erase first", workspacedeletion.TriggerErase, workspacedeletion.TriggerRestore, workspacedeletion.StateErased},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env, _ := setup(t)
			// Inside the window both moves are legal, so only the order decides.
			deletedAgo(t, env, time.Hour)
			first, second := stale(env), stale(env)

			require.NoError(t, first.FireCtx(t.Context(), c.first))
			require.ErrorIs(t, second.FireCtx(t.Context(), c.second), lifecycle.ErrInvalidTransition)

			got := workspacedeletion.StateOf(initech(t, env))
			assert.Equal(t, c.want, got)
		})
	}
}
