package instance_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/instance"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// releaseFound is a module whose Release check found 0.2.0 while 0.1.0 runs.
func releaseFound(t *testing.T, env *testhelper.TestEnv) *instance.Module {
	t.Helper()
	mod := instance.New(env.DB, instance.Environment{ReleaseCheck: true}, instance.Host{AppVersion: "0.1.0"}, nil,
		instance.WithReleaseSource(fixedSource{version: "0.2.0"}))
	require.NoError(t, mod.CheckRelease(t.Context()))
	return mod
}

// The banner of a newer release is shown to an owner of any live Workspace, not only of the
// oldest: John owns Acme, Jane owns Globex.
func TestAnOwnerOfAnyWorkspaceIsToldOfANewerRelease(t *testing.T) {
	env := testhelper.Setup(t)
	mod := releaseFound(t, env)

	john, err := mod.ReleaseFor(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	jane, err := mod.ReleaseFor(t.Context(), fixtures.OwnerJaneID)
	require.NoError(t, err)

	require.NotNil(t, john.LatestRelease)
	require.NotNil(t, jane.LatestRelease)
	assert.Equal(t, "0.2.0", *john.LatestRelease)
	assert.Equal(t, "0.2.0", *jane.LatestRelease)
}

func TestAnOwnerIsToldOfNoReleaseBeforeTheCheckFindsOne(t *testing.T) {
	env := testhelper.Setup(t)
	mod := instance.New(env.DB, instance.Environment{ReleaseCheck: true}, instance.Host{AppVersion: "0.1.0"}, nil)

	release, err := mod.ReleaseFor(t.Context(), fixtures.OwnerJohnID)

	require.NoError(t, err)
	assert.Nil(t, release.LatestRelease)
}

// Mary is a member of Acme and of Globex, Oscar belongs to no Workspace: neither holds
// release:read anywhere.
func TestSomeoneWhoOwnsNoWorkspaceIsRefusedTheRelease(t *testing.T) {
	env := testhelper.Setup(t)
	mod := releaseFound(t, env)

	_, mary := mod.ReleaseFor(t.Context(), fixtures.MemberMaryID)
	_, oscar := mod.ReleaseFor(t.Context(), fixtures.OutsiderOscarID)

	require.ErrorIs(t, mary, accounts.Refusal(accounts.PermReleaseRead))
	require.ErrorIs(t, oscar, accounts.Refusal(accounts.PermReleaseRead))
}

// An owner whose only Workspace is deleted (in its deletion window) owns no live one any more.
func TestAnOwnerOfADeletedWorkspaceOnlyIsRefusedTheRelease(t *testing.T) {
	env := testhelper.Setup(t)
	mod := releaseFound(t, env)
	require.NoError(t, env.FeedDB.Workspace.UpdateOneID(fixtures.AcmeID).SetDiscardedAt(time.Now()).Exec(t.Context()))

	_, err := mod.ReleaseFor(t.Context(), fixtures.OwnerJohnID)

	require.ErrorIs(t, err, accounts.Refusal(accounts.PermReleaseRead))
}
