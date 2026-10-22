package accounts_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestOwnersAreTheOwnerMembershipsWithTheirUsers(t *testing.T) {
	env := testhelper.Setup(t)

	owners, err := accounts.Owners(t.Context(), env.DB.Scoped(fixtures.InitechID))
	require.NoError(t, err)

	emails := lo.Map(owners, func(m *ent.Membership, _ int) string { return m.Edges.User.Email })
	assert.ElementsMatch(t, []string{fixtures.InitechOwnerOlgaEmail, fixtures.InitechCoOwnerOwenEmail}, emails)
}

func TestOwnersOfListsOwnersAcrossWorkspaces(t *testing.T) {
	env := testhelper.Setup(t)

	owners, err := accounts.OwnersOf(t.Context(), env.DB, fixtures.InitechID, fixtures.AcmeID)
	require.NoError(t, err)

	workspaces := lo.Uniq(lo.Map(owners, func(m *ent.Membership, _ int) int64 { return m.WorkspaceID }))
	assert.ElementsMatch(t, []int64{fixtures.InitechID, fixtures.AcmeID}, workspaces)
}
