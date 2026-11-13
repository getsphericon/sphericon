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

// A Workspace id another Workspace has inserts nothing: CreateIn draws again, and a generator that
// only draws taken ids is a fault after a few draws, the transaction it ran in still usable.
func TestCreateInGivesUpOnAGeneratorThatOnlyDrawsTakenIDs(t *testing.T) {
	env := testhelper.Setup(t)
	draws := 0
	taken := func() string { draws++; return fixtures.AcmePublicID }
	var err error

	txErr := env.Bus.WithinTx(t.Context(), func(tx *ent.Client, _ events.Publisher) error {
		_, err = workspacesunchecked.CreateIn(t.Context(), tx, "Fresh", taken, nil)
		return nil
	})

	require.NoError(t, txErr)
	require.ErrorContains(t, err, "every workspace id drawn is taken")
	assert.Equal(t, 3, draws, "it draws a few ids, then gives up")
}

func TestCreateInKeepsTheFirstIDThatIsFree(t *testing.T) {
	env := testhelper.Setup(t)
	ids := []string{fixtures.AcmePublicID, "FreshId1"}
	next := func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	}

	var ws *ent.Workspace
	err := env.Bus.WithinTx(t.Context(), func(tx *ent.Client, _ events.Publisher) error {
		var err error
		ws, err = workspacesunchecked.CreateIn(t.Context(), tx, "Fresh", next, nil)
		return err
	})

	require.NoError(t, err)
	assert.Equal(t, "FreshId1", ws.PublicID)
}

// erasedAt is the moment the tombstone tests erase at.
var erasedAt = time.Date(2026, time.October, 19, 12, 0, 0, 0, time.UTC)

// deletedAgo puts Initech into deletion, deleted ago before erasedAt.
func deletedAgo(t *testing.T, env *testhelper.TestEnv, ago time.Duration) {
	t.Helper()
	env.FeedDB.Workspace.UpdateOneID(fixtures.InitechID).SetDiscardedAt(erasedAt.Add(-ago)).ExecX(t.Context())
}

// tombstone runs TombstoneIn on Initech at erasedAt in a transaction of its own.
func tombstone(t *testing.T, env *testhelper.TestEnv) (bool, error) {
	t.Helper()
	var erased bool
	var err error
	txErr := env.Bus.WithinTx(t.Context(), func(tx *ent.Client, _ events.Publisher) error {
		erased, err = workspacesunchecked.TombstoneIn(t.Context(), tx, fixtures.InitechID, erasedAt)
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
		_, err := workspacesunchecked.RestoreIn(t.Context(), tx, fixtures.InitechID, erasedAt)
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
