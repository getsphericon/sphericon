package site_test

import (
	"context"
	"net/http"
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
	_, err := owner.SiteMembershipsList(ctx, siteapi.SiteMembershipsListParams{WorkspaceId: foreign})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = member.SiteMembershipsList(ctx, siteapi.SiteMembershipsListParams{WorkspaceId: acme})
	require.NoError(t, err)
	_, err = env.SiteAnonymous(t).SiteMembershipsList(ctx, siteapi.SiteMembershipsListParams{WorkspaceId: acme})
	require.Error(t, err)

	// Update.
	_, err = owner.SiteMembershipsUpdate(ctx, role, siteapi.SiteMembershipsUpdateParams{WorkspaceId: foreign, ID: memberID})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = owner.SiteMembershipsUpdate(ctx, role, siteapi.SiteMembershipsUpdateParams{WorkspaceId: acme, ID: idStr(99999)})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	// A Globex membership is invisible through Acme.
	_, err = owner.SiteMembershipsUpdate(ctx, role, siteapi.SiteMembershipsUpdateParams{WorkspaceId: acme, ID: idStr(fixtures.GlobexOwnerMembershipID)})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	// An admin may not grant the owner role.
	admin := env.SiteActor(t, fixtures.AdminAliceEmail)
	_, err = admin.SiteMembershipsUpdate(ctx, &siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleOwner},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: acme, ID: memberID})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)
	stored, err := env.DB.Membership.Get(ctx, fixtures.AcmeMemberMembershipID)
	require.NoError(t, err)
	assert.Equal(t, membership.RoleMember, stored.Role)

	// Delete.
	err = owner.SiteMembershipsDelete(ctx, siteapi.SiteMembershipsDeleteParams{WorkspaceId: foreign, ID: memberID})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	err = member.SiteMembershipsDelete(ctx, siteapi.SiteMembershipsDeleteParams{WorkspaceId: acme, ID: memberID})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)
	err = owner.SiteMembershipsDelete(ctx, siteapi.SiteMembershipsDeleteParams{WorkspaceId: acme, ID: idStr(99999)})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	err = admin.SiteMembershipsDelete(ctx, siteapi.SiteMembershipsDeleteParams{WorkspaceId: acme, ID: ownerID})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)

	// The owner removes the plain member for real.
	err = owner.SiteMembershipsDelete(ctx, siteapi.SiteMembershipsDeleteParams{WorkspaceId: acme, ID: memberID})
	require.NoError(t, err)
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
	_, err := owner.SiteInvitationsList(ctx, siteapi.SiteInvitationsListParams{WorkspaceId: foreign})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = member.SiteInvitationsList(ctx, siteapi.SiteInvitationsListParams{WorkspaceId: acme})
	require.NoError(t, err)

	// Create: foreign workspaceID 404; an existing member is a conflict.
	_, err = owner.SiteInvitationsCreate(ctx, in("x@acme.test"), siteapi.SiteInvitationsCreateParams{WorkspaceId: foreign})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = owner.SiteInvitationsCreate(ctx, in(fixtures.MemberMaryEmail), siteapi.SiteInvitationsCreateParams{WorkspaceId: acme})
	testhelper.SiteRefusal(t, err, http.StatusConflict)

	// Re-inviting the same address reissues the invitation instead of duplicating it.
	first, err := owner.SiteInvitationsCreate(ctx, in("twice@acme.test"), siteapi.SiteInvitationsCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	second, err := owner.SiteInvitationsCreate(ctx, &siteapi.SiteCreateInvitationInput{Email: "twice@acme.test", Role: siteapi.SiteInvitableRoleAdmin},
		siteapi.SiteInvitationsCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	r1, r2 := first, second
	assert.Equal(t, r1.Resource.ID, r2.Resource.ID)
	assert.NotEqual(t, r1.InviteUrl, r2.InviteUrl)
	rows, err := env.DB.Invitation.Query().Where(invitation.Email("twice@acme.test")).All(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, invitation.RoleAdmin, rows[0].Role)

	// Delete: foreign, forbidden, bad id, unknown, success.
	id := r1.Resource.ID
	err = owner.SiteInvitationsDelete(ctx, siteapi.SiteInvitationsDeleteParams{WorkspaceId: foreign, ID: id})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	err = owner.SiteInvitationsDelete(ctx, siteapi.SiteInvitationsDeleteParams{WorkspaceId: acme, ID: idStr(99999)})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	err = owner.SiteInvitationsDelete(ctx, siteapi.SiteInvitationsDeleteParams{WorkspaceId: acme, ID: id})
	require.NoError(t, err)
	left, err := env.DB.Invitation.Query().Where(invitation.Email("twice@acme.test")).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, left)
}

func TestSitePublicInvitationsRejectBadInput(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)
	ctx := context.Background()

	_, err := c.SitePublicInvitationsLookup(ctx, siteapi.SitePublicInvitationsLookupParams{Token: "no-such-token"})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	// A brand-new invitee must supply a name and password.
	err = c.SitePublicInvitationsAccept(ctx, &siteapi.SiteAcceptInvitationInput{}, siteapi.SitePublicInvitationsAcceptParams{Token: "inv_fixture_token_acme"})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	pending, err := env.DB.Invitation.Query().Where(invitation.AcceptedAtIsNil(), invitation.Email("invited@acme.test")).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, pending, "the invitation was not consumed")
}
