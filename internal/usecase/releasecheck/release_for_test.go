package releasecheck_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/instance/instancestates"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/releasecheck"
)

// releaseFound is a Release check that found 0.2.0 while 0.1.0 runs.
func releaseFound(t *testing.T, env *testhelper.TestEnv) *releasecheck.UseCase {
	t.Helper()
	states := instancestates.New(env.DB, instancestates.Environment{ReleaseCheck: true, Version: "0.1.0"}, nil)
	mod := releasecheck.New(states, fixedSource{version: "0.2.0"}, slog.Default())
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
	mod := releasecheck.New(instancestates.New(env.DB, instancestates.Environment{ReleaseCheck: true, Version: "0.1.0"}, nil), nil, slog.Default())

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

	require.ErrorIs(t, mary, memberships.Refusal(memberships.PermReleaseRead))
	require.ErrorIs(t, oscar, memberships.Refusal(memberships.PermReleaseRead))
}

// An owner whose only Workspace is deleted (in its deletion window) owns no live one any more.
func TestAnOwnerOfADeletedWorkspaceOnlyIsRefusedTheRelease(t *testing.T) {
	env := testhelper.Setup(t)
	mod := releaseFound(t, env)
	require.NoError(t, env.FeedDB.Workspace.UpdateOneID(fixtures.AcmeID).SetDiscardedAt(time.Now()).Exec(t.Context()))

	_, err := mod.ReleaseFor(t.Context(), fixtures.OwnerJohnID)

	require.ErrorIs(t, err, memberships.Refusal(memberships.PermReleaseRead))
}
