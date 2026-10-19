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

func (c workspaceCLI) SuspendWorkspace(ctx context.Context, slug string, by suspension.Actor, reason string) (bool, error) {
	id, err := accounts.WorkspaceIDBySlug(ctx, c.env.DB, slug)
	if err != nil {
		return false, err
	}
	return suspension.SuspendWorkspace(ctx, c.env.Bus, id, by, reason)
}

func (c workspaceCLI) UnsuspendWorkspace(ctx context.Context, slug string, by suspension.Actor) (bool, error) {
	id, err := accounts.WorkspaceIDBySlug(ctx, c.env.DB, slug)
	if err != nil {
		return false, err
	}
	return suspension.UnsuspendWorkspace(ctx, c.env.Bus, id, by)
}

func TestWorkspaceSuspendJoinsTheReasonWords(t *testing.T) {
	env := testhelper.Setup(t)
	var out bytes.Buffer

	err := execute(workspaceCLI{env}, &out, "workspace", "suspend", "acme", "complaint", "rate", "too", "high")

	require.NoError(t, err)

	acme := env.DB.Workspace.GetX(context.Background(), fixtures.AcmeID)
	require.NotNil(t, acme.SuspendedAt)
	require.NotNil(t, acme.SuspensionReason)
	assert.Equal(t, "complaint rate too high", *acme.SuspensionReason)
	assert.Equal(t, workspace.SuspendedByKindCli, *acme.SuspendedByKind)
	assert.Contains(t, out.String(), "suspended")
}

func TestWorkspaceSuspendNeedsASlugAndAReason(t *testing.T) {
	env := testhelper.Setup(t)
	for _, args := range [][]string{{"suspend"}, {"suspend", "acme"}} {
		err := execute(workspaceCLI{env}, &bytes.Buffer{}, append([]string{"workspace"}, args...)...)
		require.Error(t, err, "args %v", args)
	}
	acme := env.DB.Workspace.GetX(context.Background(), fixtures.AcmeID)
	assert.Nil(t, acme.SuspendedAt)
}

func TestWorkspaceUnsuspendLiftsTheSuspension(t *testing.T) {
	env := testhelper.Setup(t)
	err := execute(workspaceCLI{env}, &bytes.Buffer{}, "workspace", "suspend", "acme", "abuse")
	require.NoError(t, err)

	err = execute(workspaceCLI{env}, &bytes.Buffer{}, "workspace", "unsuspend", "acme")

	require.NoError(t, err)

	acme := env.DB.Workspace.GetX(context.Background(), fixtures.AcmeID)
	assert.Nil(t, acme.SuspendedAt)
}

func TestWorkspaceUnsuspendReportsNoChange(t *testing.T) {
	env := testhelper.Setup(t)
	var out bytes.Buffer

	err := execute(workspaceCLI{env}, &out, "workspace", "unsuspend", "acme")

	require.NoError(t, err)

	assert.Contains(t, out.String(), "not suspended")
}

func TestWorkspaceUnknownSubcommand(t *testing.T) {
	env := testhelper.Setup(t)
	err := execute(workspaceCLI{env}, &bytes.Buffer{}, "workspace", "delete", "acme")
	require.Error(t, err)
	err = execute(workspaceCLI{env}, &bytes.Buffer{}, "workspace")
	require.Error(t, err)
}
