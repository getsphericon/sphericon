package workspacesummary_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
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

// A User's summaries are one per Workspace they are a member of, each with its Membership.
func TestOfMembersSummarizesEveryWorkspaceOfTheUser(t *testing.T) {
	env := testhelper.Setup(t)

	members, err := env.WorkspaceSummary.OfMembers(t.Context(), fixtures.OwnerJohnID)

	require.NoError(t, err)
	require.Len(t, members, 1, "John is a member of Acme alone")
	assert.Equal(t, int64(fixtures.AcmeID), members[0].Membership.WorkspaceID)
	assert.Equal(t, int64(fixtures.AcmeID), members[0].Summary.Workspace.ID)
}
