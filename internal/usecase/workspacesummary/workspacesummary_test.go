package workspacesummary_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/workspacesummary"
)

// The Workspace summary is the Workspace with what its sending stands on: an active Staff hold is
// in it, and a Workspace without a Send cap has none.
func TestOfReadsTheWorkspaceWithItsHolds(t *testing.T) {
	env := testhelper.Setup(t)
	env.ExtendHold(t.Context(), fixtures.AcmeID, fixtures.HoldAcmeLapsedID, time.Now().Add(time.Hour))

	sum, err := env.WorkspaceSummary.Of(t.Context(), env.DB.Scoped(fixtures.AcmeID))

	require.NoError(t, err)
	assert.Equal(t, int64(fixtures.AcmeID), sum.Workspace.ID)
	require.Len(t, sum.Holds, 1)
	assert.Equal(t, int64(fixtures.HoldAcmeLapsedID), sum.Holds[0].ID)
	assert.Nil(t, sum.SendCap)
}

// A User's views are one per Workspace they are a member of, each with its Membership.
func TestOfMembersSummarizesEveryWorkspaceOfTheUser(t *testing.T) {
	env := testhelper.Setup(t)

	views, err := env.WorkspaceSummary.OfMembers(t.Context(), fixtures.OwnerJohnID, false)

	require.NoError(t, err)
	require.Len(t, views, 1, "John is a member of Acme alone")
	assert.Equal(t, int64(fixtures.AcmeID), views[0].Membership.WorkspaceID)
	assert.Equal(t, int64(fixtures.AcmeID), views[0].Summary.Workspace.ID)
}

// Umbrella requires a Second factor since 2026-03-01: Rita has none, so her grace ends seven days
// later in it, and Initech, which requires none, gives her no grace to count down.
var ritaGraceEnds = time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)

// graceOf is the grace end of the view of the Workspace workspaceID among views, nil for none.
func graceOf(t *testing.T, views []workspacesummary.View, workspaceID int64) *time.Time {
	t.Helper()
	for _, v := range views {
		if v.Membership.WorkspaceID == workspaceID {
			return v.SecondFactorGraceEndsAt
		}
	}
	require.Failf(t, "no view", "of the Workspace %d", workspaceID)
	return nil
}

// The view carries the caller's grace under each Workspace's Two-factor requirement.
func TestOfMembersCarriesTheGraceUnderTheRequirement(t *testing.T) {
	env := testhelper.Setup(t)

	views, err := env.WorkspaceSummary.OfMembers(t.Context(), fixtures.UmbrellaOwnerRitaID, false)

	require.NoError(t, err)
	umbrella := graceOf(t, views, fixtures.UmbrellaID)
	require.NotNil(t, umbrella)
	assert.True(t, umbrella.Equal(ritaGraceEnds), "got %s", umbrella)
	assert.Nil(t, graceOf(t, views, fixtures.InitechID), "a Workspace without the requirement")
}

// A strong session meets the requirement by itself (secondfactors.Module.Admit), so it has no grace
// to count down.
func TestOfMembersGivesAStrongSessionNoGrace(t *testing.T) {
	env := testhelper.Setup(t)

	views, err := env.WorkspaceSummary.OfMembers(t.Context(), fixtures.UmbrellaOwnerRitaID, true)

	require.NoError(t, err)
	assert.Nil(t, graceOf(t, views, fixtures.UmbrellaID))
}

// Switching the requirement on answers the caller's View with their grace, which starts with it:
// the grace is read on the Workspace as the change left it, not as the caller's Membership was
// loaded.
func TestSwitchingTheRequirementOnStartsTheCallersGrace(t *testing.T) {
	at := time.Now().UTC().Truncate(time.Second)
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
	s, m, err := memberships.New(env.DB, env.Bus).Scope(t.Context(), fixtures.UmbrellaOwnerRitaID, fixtures.InitechPublicID)
	require.NoError(t, err)

	v, err := env.WorkspaceSummary.SetSecondFactorRequirement(t.Context(), s, workspacesummary.Caller{Membership: m}, true)

	require.NoError(t, err)
	require.NotNil(t, v.Summary.Workspace.SecondFactorRequiredAt)
	require.NotNil(t, v.SecondFactorGraceEndsAt, "Rita has no Second factor")
	assert.True(t, v.SecondFactorGraceEndsAt.Equal(at.Add(secondfactors.GracePeriod)), "got %s", v.SecondFactorGraceEndsAt)
}
