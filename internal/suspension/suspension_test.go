package suspension_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mokevnin/sphericon/ent/workspace"
	"github.com/mokevnin/sphericon/internal/fixtures"
	"github.com/mokevnin/sphericon/internal/suspension"
	"github.com/mokevnin/sphericon/internal/testhelper"
)

func TestSuspendWorkspaceRecordsAttribution(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	changed, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.CLI, "complaint rate 0.9%")
	require.NoError(t, err)
	assert.True(t, changed)

	ws := env.DB.Workspace.GetX(ctx, fixtures.AcmeID)
	require.NotNil(t, ws.SuspendedAt)
	require.NotNil(t, ws.SuspendedByKind)
	assert.Equal(t, workspace.SuspendedByKindCli, *ws.SuspendedByKind)
	assert.Nil(t, ws.SuspendedByID, "only an Operator carries an id")
	require.NotNil(t, ws.SuspensionReason)
	assert.Equal(t, "complaint rate 0.9%", *ws.SuspensionReason)
}

// Suspending an already-suspended workspace changes nothing, so the first actor and
// reason (the ones the owner was told about) are never overwritten.
func TestSuspendWorkspaceIsIdempotent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.System, "first")
	require.NoError(t, err)
	changed, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.CLI, "second")
	require.NoError(t, err)
	assert.False(t, changed)

	ws := env.DB.Workspace.GetX(ctx, fixtures.AcmeID)
	assert.Equal(t, workspace.SuspendedByKindSystem, *ws.SuspendedByKind)
	assert.Equal(t, "first", *ws.SuspensionReason)
}

func TestUnsuspendWorkspaceClearsEverything(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.CLI, "abuse")
	require.NoError(t, err)
	changed, err := suspension.UnsuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.CLI)
	require.NoError(t, err)
	assert.True(t, changed)

	ws := env.DB.Workspace.GetX(ctx, fixtures.AcmeID)
	assert.Nil(t, ws.SuspendedAt)
	assert.Nil(t, ws.SuspendedByKind)
	assert.Nil(t, ws.SuspendedByID)
	assert.Nil(t, ws.SuspensionReason)

	changed, err = suspension.UnsuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.CLI)
	require.NoError(t, err)
	assert.False(t, changed, "unsuspending a workspace that is not suspended is a no-op")
}

func TestSuspendWorkspaceRequiresAReason(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := suspension.SuspendWorkspace(context.Background(), env.Bus, fixtures.AcmeID, suspension.CLI, "  ")
	require.Error(t, err, "attribution needs a reason: the owner is told why")
}

// An Operator's real id is kept on the internal record; it is the only actor with one.
func TestSuspendWorkspaceKeepsTheOperatorId(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.Operator("op-42"), "abuse report")
	require.NoError(t, err)

	ws := env.DB.Workspace.GetX(ctx, fixtures.AcmeID)
	require.NotNil(t, ws.SuspendedByKind)
	assert.Equal(t, workspace.SuspendedByKindOperator, *ws.SuspendedByKind)
	require.NotNil(t, ws.SuspendedByID)
	assert.Equal(t, "op-42", *ws.SuspendedByID)
}

func TestSuspendWorkspaceRejectsAnInvalidActor(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.Operator(""), "abuse")
	require.Error(t, err, "an Operator without an id")
	_, err = suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.Actor{}, "abuse")
	require.Error(t, err, "no actor")
	_, err = suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.Actor{Kind: workspace.SuspendedByKindSystem, ID: "x"}, "abuse")
	require.Error(t, err, "only an Operator has an id")
}

// A Workspace that was suspended before the test starts (fixture Hooli) is lifted and
// its structured actor cleared.
func TestUnsuspendFixtureWorkspaceClearsTheStructuredActor(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	ws := env.DB.Workspace.GetX(ctx, fixtures.HooliID)
	require.NotNil(t, ws.SuspendedByKind)
	require.Equal(t, workspace.SuspendedByKindOperator, *ws.SuspendedByKind)

	changed, err := suspension.UnsuspendWorkspace(ctx, env.Bus, fixtures.HooliID, suspension.CLI)
	require.NoError(t, err)
	assert.True(t, changed)
	ws = env.DB.Workspace.GetX(ctx, fixtures.HooliID)
	assert.Nil(t, ws.SuspendedByKind)
	assert.Nil(t, ws.SuspendedByID)
}
