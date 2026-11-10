package site_test

import (
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
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

	err := mary.SiteWorkspacesLeave(t.Context(), siteapi.SiteWorkspacesLeaveParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	assert.Equal(t, []siteapi.WorkspaceId{fixtures.GlobexPublicID}, workspaceIDs(t, mary))
}

func TestLeavingAsAdminRemovesTheMembershipFromTheMembersList(t *testing.T) {
	env := testhelper.Setup(t)
	alice := env.SiteActor(t, fixtures.AdminAliceEmail)

	err := alice.SiteWorkspacesLeave(t.Context(), siteapi.SiteWorkspacesLeaveParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	items, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteMembershipsList(t.Context(), siteapi.SiteMembershipsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	emails := lo.Map(items, func(m siteapi.SiteMembershipResource, _ int) siteapi.EmailAddress { return m.Email })
	assert.NotContains(t, emails, siteapi.EmailAddress(fixtures.AdminAliceEmail))
}

func TestLeavingAsACoOwnerKeepsTheOtherOwner(t *testing.T) {
	env := testhelper.Setup(t)
	owen := env.SiteActor(t, fixtures.InitechCoOwnerOwenEmail)

	err := owen.SiteWorkspacesLeave(t.Context(), siteapi.SiteWorkspacesLeaveParams{WorkspaceId: fixtures.InitechPublicID})
	require.NoError(t, err)

	olga := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)
	assert.Contains(t, workspaceIDs(t, olga), siteapi.WorkspaceId(fixtures.InitechPublicID))
}

func TestLeavingAsTheOnlyOwnerIsRefusedAndNothingIsRemoved(t *testing.T) {
	env := testhelper.Setup(t)
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)

	err := john.SiteWorkspacesLeave(t.Context(), siteapi.SiteWorkspacesLeaveParams{WorkspaceId: fixtures.AcmePublicID})
	refused := testhelper.SiteRefusal(t, err, http.StatusConflict)
	assert.Contains(t, refused.Errors.Value["workspace"][0], "another member an owner")

	assert.Equal(t, []siteapi.WorkspaceId{fixtures.AcmePublicID}, workspaceIDs(t, john))
}

func TestLeavingAWorkspaceTheUserDoesNotBelongToIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)

	err := john.SiteWorkspacesLeave(t.Context(), siteapi.SiteWorkspacesLeaveParams{WorkspaceId: fixtures.GlobexPublicID})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}

func TestLeavingIsAuditedWithTheUserAsActor(t *testing.T) {
	env := testhelper.Setup(t)
	err := env.SiteActor(t, fixtures.MemberMaryEmail).SiteWorkspacesLeave(t.Context(), siteapi.SiteWorkspacesLeaveParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)

	got := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionMembershipDelete)
	require.Len(t, got, 1)
	assert.Equal(t, siteapi.SiteAuditActorKindUser, got[0].Actor.Kind)
	assert.Equal(t, fixtures.MemberMaryName, got[0].Actor.Name.Value)
}
