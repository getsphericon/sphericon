package suspension_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSuspendWorkspaceRecordsAttribution(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	changed, err := testhelper.Module[*suspension.Suspender](env).Suspend(ctx, fixtures.AcmeID, suspension.SuspendInput{By: suspension.CLI, Reason: "complaint rate 0.9%"})
	require.NoError(t, err)
	assert.True(t, changed)

	ws := env.DB.Workspace.GetX(ctx, fixtures.AcmeID)
	require.NotNil(t, ws.SuspendedAt)
	require.NotNil(t, ws.SuspendedByKind)
	assert.Equal(t, workspace.SuspendedByKindCli, *ws.SuspendedByKind)
	assert.Nil(t, ws.SuspendedByID, "only a StaffMember carries an id")
	require.NotNil(t, ws.SuspensionReason)
	assert.Equal(t, "complaint rate 0.9%", *ws.SuspensionReason)
}

// A suspension tells the owners why, so one without a reason is refused and changes nothing,
// whoever asks.
func TestSuspendWorkspaceRequiresAReason(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	changed, err := testhelper.Module[*suspension.Suspender](env).Suspend(ctx, fixtures.AcmeID, suspension.SuspendInput{By: suspension.CLI, Reason: "   "})

	require.ErrorIs(t, err, suspension.ErrReasonRequired)
	assert.False(t, changed)
	assert.Nil(t, env.DB.Workspace.GetX(ctx, fixtures.AcmeID).SuspendedAt)
}

// Suspending an already-suspended workspace changes nothing, so the first actor and
// reason (the ones the owner was told about) are never overwritten.
func TestSuspendWorkspaceIsIdempotent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	_, err := testhelper.Module[*suspension.Suspender](env).Suspend(ctx, fixtures.AcmeID, suspension.SuspendInput{By: suspension.System, Reason: "first"})
	require.NoError(t, err)
	changed, err := testhelper.Module[*suspension.Suspender](env).Suspend(ctx, fixtures.AcmeID, suspension.SuspendInput{By: suspension.CLI, Reason: "second"})
	require.NoError(t, err)
	assert.False(t, changed)

	ws := env.DB.Workspace.GetX(ctx, fixtures.AcmeID)
	assert.Equal(t, workspace.SuspendedByKindSystem, *ws.SuspendedByKind)
	assert.Equal(t, "first", *ws.SuspensionReason)
}

func TestUnsuspendWorkspaceClearsEverything(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	_, err := testhelper.Module[*suspension.Suspender](env).Suspend(ctx, fixtures.AcmeID, suspension.SuspendInput{By: suspension.CLI, Reason: "abuse"})
	require.NoError(t, err)
	changed, err := testhelper.Module[*suspension.Suspender](env).Unsuspend(ctx, fixtures.AcmeID, suspension.CLI)
	require.NoError(t, err)
	assert.True(t, changed)

	ws := env.DB.Workspace.GetX(ctx, fixtures.AcmeID)
	assert.Nil(t, ws.SuspendedAt)
	assert.Nil(t, ws.SuspendedByKind)
	assert.Nil(t, ws.SuspendedByID)
	assert.Nil(t, ws.SuspensionReason)

	changed, err = testhelper.Module[*suspension.Suspender](env).Unsuspend(ctx, fixtures.AcmeID, suspension.CLI)
	require.NoError(t, err)
	assert.False(t, changed, "unsuspending a workspace that is not suspended is a no-op")
}

// A Staff member's real id is kept on the internal record; it is the only actor with one.
func TestSuspendWorkspaceKeepsTheStaffId(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	_, err := testhelper.Module[*suspension.Suspender](env).Suspend(ctx, fixtures.AcmeID, suspension.SuspendInput{By: suspension.StaffMember("op-42"), Reason: "abuse report"})
	require.NoError(t, err)

	ws := env.DB.Workspace.GetX(ctx, fixtures.AcmeID)
	require.NotNil(t, ws.SuspendedByKind)
	assert.Equal(t, workspace.SuspendedByKindStaff, *ws.SuspendedByKind)
	require.NotNil(t, ws.SuspendedByID)
	assert.Equal(t, "op-42", *ws.SuspendedByID)
}

func TestSuspendWorkspaceRejectsAnInvalidActor(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	_, err := testhelper.Module[*suspension.Suspender](env).Suspend(ctx, fixtures.AcmeID, suspension.SuspendInput{By: suspension.StaffMember(""), Reason: "abuse"})
	require.Error(t, err, "a StaffMember without an id")
	_, err = testhelper.Module[*suspension.Suspender](env).Suspend(ctx, fixtures.AcmeID, suspension.SuspendInput{By: suspension.Actor{}, Reason: "abuse"})
	require.Error(t, err, "no actor")
	_, err = testhelper.Module[*suspension.Suspender](env).Suspend(ctx, fixtures.AcmeID, suspension.SuspendInput{By: suspension.Actor{Kind: workspace.SuspendedByKindSystem, ID: "x"}, Reason: "abuse"})
	require.Error(t, err, "only a StaffMember has an id")
}

// A Workspace that was suspended before the test starts (fixture Hooli) is lifted and
// its structured actor cleared.
func TestUnsuspendFixtureWorkspaceClearsTheStructuredActor(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	ws := env.DB.Workspace.GetX(ctx, fixtures.HooliID)
	require.NotNil(t, ws.SuspendedByKind)
	require.Equal(t, workspace.SuspendedByKindStaff, *ws.SuspendedByKind)

	changed, err := testhelper.Module[*suspension.Suspender](env).Unsuspend(ctx, fixtures.HooliID, suspension.CLI)
	require.NoError(t, err)
	assert.True(t, changed)
	ws = env.DB.Workspace.GetX(ctx, fixtures.HooliID)
	assert.Nil(t, ws.SuspendedByKind)
	assert.Nil(t, ws.SuspendedByID)
}

func TestSuspendedPredicatesFollowTheSuspension(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	_, err := testhelper.Module[*suspension.Suspender](env).Suspend(ctx, fixtures.AcmeID, suspension.SuspendInput{By: suspension.System, Reason: "complaint rate 0.9%"})
	require.NoError(t, err)

	suspended, err := env.DB.Workspace.Query().Where(suspension.Suspended(), workspace.ID(fixtures.AcmeID)).IDs(ctx)
	require.NoError(t, err)
	bySystem, err := env.DB.Workspace.Query().Where(suspension.BySystem(), workspace.ID(fixtures.AcmeID)).IDs(ctx)
	require.NoError(t, err)
	notSuspended, err := env.DB.Workspace.Query().Where(suspension.NotSuspended(), workspace.ID(fixtures.AcmeID)).Count(ctx)
	require.NoError(t, err)

	assert.Equal(t, []int64{fixtures.AcmeID}, suspended)
	assert.Equal(t, []int64{fixtures.AcmeID}, bySystem)
	assert.True(t, suspension.Is(env.DB.Workspace.GetX(ctx, fixtures.AcmeID)))
	assert.Zero(t, notSuspended)
}
