package site_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func workspaceIDs(t *testing.T, c *siteapi.Client) []siteapi.WorkspaceId {
	t.Helper()
	list, err := c.SiteWorkspacesList(t.Context())
	require.NoError(t, err)
	return lo.Map(list, func(w siteapi.SiteWorkspaceResource, _ int) siteapi.WorkspaceId { return w.ID })
}

func TestLeavingAsMemberRemovesTheWorkspaceFromTheirList(t *testing.T) {
	env := testhelper.Setup(t)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)

	res, err := mary.SiteWorkspacesLeave(t.Context(), siteapi.SiteWorkspacesLeaveParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWorkspacesLeaveNoContent{}, res)

	assert.Equal(t, []siteapi.WorkspaceId{fixtures.GlobexPublicID}, workspaceIDs(t, mary))
}

func TestLeavingAsAdminRemovesTheMembershipFromTheMembersList(t *testing.T) {
	env := testhelper.Setup(t)
	alice := env.SiteActor(t, fixtures.AdminAliceEmail)

	res, err := alice.SiteWorkspacesLeave(t.Context(), siteapi.SiteWorkspacesLeaveParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWorkspacesLeaveNoContent{}, res)

	items, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteMembershipsList(t.Context(), siteapi.SiteMembershipsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	members, ok := items.(*siteapi.SiteMembershipsListOKApplicationJSON)
	require.True(t, ok)
	emails := lo.Map(*members, func(m siteapi.SiteMembershipResource, _ int) siteapi.EmailAddress { return m.Email })
	assert.NotContains(t, emails, siteapi.EmailAddress(fixtures.AdminAliceEmail))
}

func TestLeavingAsACoOwnerKeepsTheOtherOwner(t *testing.T) {
	env := testhelper.Setup(t)
	owen := env.SiteActor(t, fixtures.InitechCoOwnerOwenEmail)

	res, err := owen.SiteWorkspacesLeave(t.Context(), siteapi.SiteWorkspacesLeaveParams{WorkspaceId: fixtures.InitechPublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWorkspacesLeaveNoContent{}, res)

	olga := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)
	assert.Contains(t, workspaceIDs(t, olga), siteapi.WorkspaceId(fixtures.InitechPublicID))
}

func TestLeavingAsTheOnlyOwnerIsRefusedAndNothingIsRemoved(t *testing.T) {
	env := testhelper.Setup(t)
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := john.SiteWorkspacesLeave(t.Context(), siteapi.SiteWorkspacesLeaveParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	refused, ok := res.(*siteapi.SiteWorkspacesLeaveUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, refused.Errors.Value["workspace"][0], "another member an owner")

	assert.Equal(t, []siteapi.WorkspaceId{fixtures.AcmePublicID}, workspaceIDs(t, john))
}

func TestLeavingAWorkspaceTheUserDoesNotBelongToIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := john.SiteWorkspacesLeave(t.Context(), siteapi.SiteWorkspacesLeaveParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWorkspacesLeaveNotFound{}, res)
}

func TestLeavingIsAuditedWithTheUserAsActor(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := env.SiteActor(t, fixtures.MemberMaryEmail).SiteWorkspacesLeave(t.Context(), siteapi.SiteWorkspacesLeaveParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	env.DeliverToEE(t)

	got := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionMembershipDelete)
	require.Len(t, got, 1)
	assert.Equal(t, siteapi.SiteAuditActorKindUser, got[0].Actor.Kind)
	assert.Equal(t, fixtures.MemberMaryName, got[0].Actor.Name.Value)
}
