package site_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/invitation"
	"github.com/getsphericon/sphericon/ent/membership"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSiteMembershipsErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	ctx := context.Background()
	acme, foreign := siteapi.WorkspaceId(fixtures.AcmePublicID), siteapi.WorkspaceId(fixtures.GlobexPublicID)
	memberID := idStr(fixtures.AcmeMemberMembershipID)
	ownerID := idStr(fixtures.AcmeOwnerMembershipID)
	role := &siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleAdmin}

	// List: foreign workspaceID is a 404, a plain member may list, anonymous is rejected.
	nf, err := owner.SiteMembershipsList(ctx, siteapi.SiteMembershipsListParams{WorkspaceId: foreign})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteMembershipsListNotFound{}, nf)
	listed, err := member.SiteMembershipsList(ctx, siteapi.SiteMembershipsListParams{WorkspaceId: acme})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteMembershipsListOKApplicationJSON{}, listed)
	_, err = env.SiteAnonymous(t).SiteMembershipsList(ctx, siteapi.SiteMembershipsListParams{WorkspaceId: acme})
	require.Error(t, err)

	// Update.
	u1, err := owner.SiteMembershipsUpdate(ctx, role, siteapi.SiteMembershipsUpdateParams{WorkspaceId: foreign, ID: memberID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteMembershipsUpdateNotFound{}, u1)
	u3, err := owner.SiteMembershipsUpdate(ctx, role, siteapi.SiteMembershipsUpdateParams{WorkspaceId: acme, ID: idStr(99999)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteMembershipsUpdateNotFound{}, u3)
	// A Globex membership is invisible through Acme.
	u4, err := owner.SiteMembershipsUpdate(ctx, role, siteapi.SiteMembershipsUpdateParams{WorkspaceId: acme, ID: idStr(fixtures.GlobexOwnerMembershipID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteMembershipsUpdateNotFound{}, u4)

	// An admin may not grant the owner role.
	admin := env.SiteActor(t, fixtures.AdminAliceEmail)
	u5, err := admin.SiteMembershipsUpdate(ctx, &siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleOwner},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: acme, ID: memberID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteMembershipsUpdateForbidden{}, u5)
	stored, err := env.DB.Membership.Get(ctx, fixtures.AcmeMemberMembershipID)
	require.NoError(t, err)
	assert.Equal(t, membership.RoleMember, stored.Role)

	// Delete.
	d1, err := owner.SiteMembershipsDelete(ctx, siteapi.SiteMembershipsDeleteParams{WorkspaceId: foreign, ID: memberID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteMembershipsDeleteNotFound{}, d1)
	d2, err := member.SiteMembershipsDelete(ctx, siteapi.SiteMembershipsDeleteParams{WorkspaceId: acme, ID: memberID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteMembershipsDeleteForbidden{}, d2)
	d4, err := owner.SiteMembershipsDelete(ctx, siteapi.SiteMembershipsDeleteParams{WorkspaceId: acme, ID: idStr(99999)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteMembershipsDeleteNotFound{}, d4)
	d5, err := admin.SiteMembershipsDelete(ctx, siteapi.SiteMembershipsDeleteParams{WorkspaceId: acme, ID: ownerID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteMembershipsDeleteForbidden{}, d5)

	// The owner removes the plain member for real.
	d6, err := owner.SiteMembershipsDelete(ctx, siteapi.SiteMembershipsDeleteParams{WorkspaceId: acme, ID: memberID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteMembershipsDeleteNoContent{}, d6)
	exists, err := env.DB.Membership.Query().Where(membership.ID(fixtures.AcmeMemberMembershipID)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestSiteInvitationsErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	ctx := context.Background()
	acme, foreign := siteapi.WorkspaceId(fixtures.AcmePublicID), siteapi.WorkspaceId(fixtures.GlobexPublicID)
	in := func(email string) *siteapi.SiteCreateInvitationInput {
		return &siteapi.SiteCreateInvitationInput{Email: siteapi.EmailAddress(email), Role: siteapi.SiteInvitableRoleMember}
	}

	// List: scoped and authenticated; any member may read it.
	nf, err := owner.SiteInvitationsList(ctx, siteapi.SiteInvitationsListParams{WorkspaceId: foreign})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteInvitationsListNotFound{}, nf)
	ok, err := member.SiteInvitationsList(ctx, siteapi.SiteInvitationsListParams{WorkspaceId: acme})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteInvitationsListOKApplicationJSON{}, ok)

	// Create: foreign workspaceID 404; an existing member is a conflict.
	c1, err := owner.SiteInvitationsCreate(ctx, in("x@acme.test"), siteapi.SiteInvitationsCreateParams{WorkspaceId: foreign})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteInvitationsCreateNotFound{}, c1)
	c2, err := owner.SiteInvitationsCreate(ctx, in(fixtures.MemberMaryEmail), siteapi.SiteInvitationsCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteInvitationsCreateConflict{}, c2)

	// Re-inviting the same address reissues the invitation instead of duplicating it.
	first, err := owner.SiteInvitationsCreate(ctx, in("twice@acme.test"), siteapi.SiteInvitationsCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	second, err := owner.SiteInvitationsCreate(ctx, &siteapi.SiteCreateInvitationInput{Email: "twice@acme.test", Role: siteapi.SiteInvitableRoleAdmin},
		siteapi.SiteInvitationsCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	r1, r2 := first.(*siteapi.SiteCreateInvitationResponse), second.(*siteapi.SiteCreateInvitationResponse)
	assert.Equal(t, r1.Resource.ID, r2.Resource.ID)
	assert.NotEqual(t, r1.InviteUrl, r2.InviteUrl)
	rows, err := env.DB.Invitation.Query().Where(invitation.Email("twice@acme.test")).All(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, invitation.RoleAdmin, rows[0].Role)

	// Delete: foreign, forbidden, bad id, unknown, success.
	id := r1.Resource.ID
	d1, err := owner.SiteInvitationsDelete(ctx, siteapi.SiteInvitationsDeleteParams{WorkspaceId: foreign, ID: id})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteInvitationsDeleteNotFound{}, d1)
	d3, err := owner.SiteInvitationsDelete(ctx, siteapi.SiteInvitationsDeleteParams{WorkspaceId: acme, ID: idStr(99999)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteInvitationsDeleteNotFound{}, d3)
	d4, err := owner.SiteInvitationsDelete(ctx, siteapi.SiteInvitationsDeleteParams{WorkspaceId: acme, ID: id})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteInvitationsDeleteNoContent{}, d4)
	left, err := env.DB.Invitation.Query().Where(invitation.Email("twice@acme.test")).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, left)
}

func TestSitePublicInvitationsRejectBadInput(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)
	ctx := context.Background()

	unknown, err := c.SitePublicInvitationsLookup(ctx, siteapi.SitePublicInvitationsLookupParams{Token: "no-such-token"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SitePublicInvitationsLookupNotFound{}, unknown)

	// A brand-new invitee must supply a name and password.
	res, err := c.SitePublicInvitationsAccept(ctx, &siteapi.SiteAcceptInvitationInput{}, siteapi.SitePublicInvitationsAcceptParams{Token: "inv_fixture_token_acme"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SitePublicInvitationsAcceptUnprocessableEntity{}, res)
	pending, err := env.DB.Invitation.Query().Where(invitation.AcceptedAtIsNil(), invitation.Email("invited@acme.test")).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, pending, "the invitation was not consumed")
}
