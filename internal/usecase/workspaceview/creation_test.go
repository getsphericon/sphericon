package workspaceview_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/workspaceid"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/workspaceview"
)

// A created Workspace has a Workspace id, and its creator reads it as its Owner.
func TestCreateAnswersTheWorkspaceItsOwnerReads(t *testing.T) {
	env := testhelper.Setup(t)

	v, err := env.WorkspaceView.Create(t.Context(), workspaceview.CreateInput{UserID: fixtures.MemberMaryID, Name: " Fresh "})
	require.NoError(t, err)

	assert.Regexp(t, workspaceid.Pattern, v.Summary.Workspace.PublicID)
	assert.Equal(t, "Fresh", v.Summary.Workspace.Name, "the name is trimmed")
	assert.Equal(t, membership.RoleOwner, v.Membership.Role)
	assert.Equal(t, int64(fixtures.MemberMaryID), v.Membership.UserID)
	assert.Nil(t, v.SecondFactorGraceEndsAt, "a new Workspace has no Two-factor requirement")
}

func TestCreateRefusesAnEmptyName(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.WorkspaceView.Create(t.Context(), workspaceview.CreateInput{UserID: fixtures.MemberMaryID, Name: "  "})

	require.ErrorIs(t, err, workspaces.ErrNameEmpty)
}

func TestCreateRefusesAnUnverifiedUser(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.WorkspaceView.Create(t.Context(), workspaceview.CreateInput{UserID: fixtures.FreshUnverifiedUserID, Name: "Fresh"})

	require.ErrorIs(t, err, users.ErrEmailNotVerified)
}
