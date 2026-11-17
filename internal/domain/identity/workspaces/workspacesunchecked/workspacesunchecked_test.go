package workspacesunchecked_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces/workspacesunchecked"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// erasedAt is the moment the tombstone tests erase at.
var erasedAt = time.Date(2026, time.October, 19, 12, 0, 0, 0, time.UTC)

// unchecked runs the moves at erasedAt.
var unchecked = workspacesunchecked.New(func() time.Time { return erasedAt })

// deletedAgo puts Initech into deletion, deleted ago before erasedAt.
func deletedAgo(t *testing.T, env *testhelper.TestEnv, ago time.Duration) {
	t.Helper()
	env.DeleteWorkspace(t, fixtures.InitechID, erasedAt.Add(-ago))
}

// tombstone runs TombstoneIn on Initech at erasedAt in a transaction of its own.
func tombstone(t *testing.T, env *testhelper.TestEnv) (bool, error) {
	t.Helper()
	var erased bool
	var err error
	txErr := env.Bus.WithinTx(t.Context(), func(tx *ent.Client, _ events.Publisher) error {
		erased, err = unchecked.TombstoneIn(t.Context(), tx, fixtures.InitechID)
		return nil
	})
	require.NoError(t, txErr)
	return erased, err
}

// A Workspace deleted a day ago is still inside its 30-day window: its tombstone is refused and
// it keeps its name, so its Owner can still restore it.
func TestTombstoneInRefusesAWorkspaceInsideItsWindow(t *testing.T) {
	env := testhelper.Setup(t)
	deletedAgo(t, env, 24*time.Hour)
	name := env.DB.Workspace.GetX(t.Context(), fixtures.InitechID).Name

	erased, err := tombstone(t, env)

	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
	require.ErrorContains(t, err, "inside its deletion window")
	assert.False(t, erased)
	got := env.DB.Workspace.GetX(t.Context(), fixtures.InitechID)
	assert.Nil(t, got.ErasedAt)
	assert.Equal(t, name, got.Name)
}

// A Workspace restored before its erase reached the tombstone is no longer pending deletion: the
// erase is finished with nothing written, not refused, and the Workspace keeps its name.
func TestTombstoneInFinishesAWorkspaceRestoredFirstAsANoOp(t *testing.T) {
	env := testhelper.Setup(t)
	deletedAgo(t, env, 24*time.Hour)
	require.NoError(t, env.Bus.WithinTx(t.Context(), func(tx *ent.Client, _ events.Publisher) error {
		_, err := unchecked.RestoreIn(t.Context(), tx, fixtures.InitechID)
		return err
	}))
	before := env.DB.Workspace.GetX(t.Context(), fixtures.InitechID)

	erased, err := tombstone(t, env)

	require.NoError(t, err)
	assert.False(t, erased)
	got := env.DB.Workspace.GetX(t.Context(), fixtures.InitechID)
	assert.Nil(t, got.ErasedAt)
	assert.Equal(t, before.Name, got.Name)
	assert.Equal(t, before.CollectKey, got.CollectKey)
}

// A Workspace deleted 31 days ago is past its 30-day window: the tombstone is written, with
// nothing that names the Workspace left on its row.
func TestTombstoneInErasesAWorkspacePastItsWindow(t *testing.T) {
	env := testhelper.Setup(t)
	deletedAgo(t, env, 31*24*time.Hour)

	erased, err := tombstone(t, env)

	require.NoError(t, err)
	assert.True(t, erased)
	got := env.DB.Workspace.GetX(t.Context(), fixtures.InitechID)
	assert.Equal(t, erasedAt, got.ErasedAt.UTC())
	assert.Empty(t, got.Name)
}

// A second erase of a Workspace erased already finds nothing pending: it is a no-op, and the
// tombstone keeps the moment it was first written.
func TestTombstoneInFinishesAWorkspaceErasedAlreadyAsANoOp(t *testing.T) {
	env := testhelper.Setup(t)
	deletedAgo(t, env, 31*24*time.Hour)
	_, err := tombstone(t, env)
	require.NoError(t, err)
	first := env.DB.Workspace.GetX(t.Context(), fixtures.InitechID)

	erased, err := tombstone(t, env)

	require.NoError(t, err)
	assert.False(t, erased)
	got := env.DB.Workspace.GetX(t.Context(), fixtures.InitechID)
	assert.Equal(t, first.ErasedAt, got.ErasedAt)
	assert.Equal(t, first.CollectKey, got.CollectKey, "no fresh collect key is drawn")
}
