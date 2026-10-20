package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// workspaceCLI is the real suspension logic behind the `workspace` commands, so a test observes
// the Workspace row the command left behind.
type workspaceCLI struct{ env *testhelper.TestEnv }

func (c workspaceCLI) SuspendWorkspace(ctx context.Context, publicID string, by suspension.Actor, reason string) (bool, error) {
	id, err := accounts.WorkspaceIDByPublicID(ctx, c.env.DB, publicID)
	if err != nil {
		return false, err
	}
	return suspension.SuspendWorkspace(ctx, c.env.Bus, id, by, reason)
}

func (c workspaceCLI) UnsuspendWorkspace(ctx context.Context, publicID string, by suspension.Actor) (bool, error) {
	id, err := accounts.WorkspaceIDByPublicID(ctx, c.env.DB, publicID)
	if err != nil {
		return false, err
	}
	return suspension.UnsuspendWorkspace(ctx, c.env.Bus, id, by)
}

func TestWorkspaceSuspendJoinsTheReasonWords(t *testing.T) {
	env := testhelper.Setup(t)
	var out bytes.Buffer

	err := execute(workspaceCLI{env}, &out, "workspace", "suspend", fixtures.AcmePublicID, "complaint", "rate", "too", "high")

	require.NoError(t, err)

	acme := env.DB.Workspace.GetX(context.Background(), fixtures.AcmeID)
	require.NotNil(t, acme.SuspendedAt)
	require.NotNil(t, acme.SuspensionReason)
	assert.Equal(t, "complaint rate too high", *acme.SuspensionReason)
	assert.Equal(t, workspace.SuspendedByKindCli, *acme.SuspendedByKind)
	assert.Contains(t, out.String(), "suspended")
}

func TestWorkspaceSuspendNeedsAnIdAndAReason(t *testing.T) {
	env := testhelper.Setup(t)
	for _, args := range [][]string{{"suspend"}, {"suspend", fixtures.AcmePublicID}} {
		err := execute(workspaceCLI{env}, &bytes.Buffer{}, append([]string{"workspace"}, args...)...)
		require.Error(t, err, "args %v", args)
	}
	acme := env.DB.Workspace.GetX(context.Background(), fixtures.AcmeID)
	assert.Nil(t, acme.SuspendedAt)
}

func TestWorkspaceUnsuspendLiftsTheSuspension(t *testing.T) {
	env := testhelper.Setup(t)
	err := execute(workspaceCLI{env}, &bytes.Buffer{}, "workspace", "suspend", fixtures.AcmePublicID, "abuse")
	require.NoError(t, err)

	err = execute(workspaceCLI{env}, &bytes.Buffer{}, "workspace", "unsuspend", fixtures.AcmePublicID)

	require.NoError(t, err)

	acme := env.DB.Workspace.GetX(context.Background(), fixtures.AcmeID)
	assert.Nil(t, acme.SuspendedAt)
}

func TestWorkspaceUnsuspendReportsNoChange(t *testing.T) {
	env := testhelper.Setup(t)
	var out bytes.Buffer

	err := execute(workspaceCLI{env}, &out, "workspace", "unsuspend", fixtures.AcmePublicID)

	require.NoError(t, err)

	assert.Contains(t, out.String(), "not suspended")
}

func TestWorkspaceUnknownSubcommand(t *testing.T) {
	env := testhelper.Setup(t)
	err := execute(workspaceCLI{env}, &bytes.Buffer{}, "workspace", "delete", fixtures.AcmePublicID)
	require.Error(t, err)
	err = execute(workspaceCLI{env}, &bytes.Buffer{}, "workspace")
	require.Error(t, err)
}
