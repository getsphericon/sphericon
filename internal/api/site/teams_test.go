package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/membership"
	entuser "github.com/getsphericon/sphericon/ent/user"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSiteInvitationsCreateAndList(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail) // owner
	ctx := context.Background()

	created, err := c.SiteInvitationsCreate(ctx,
		&siteapi.SiteCreateInvitationInput{Email: "newbie@acme.test", Role: siteapi.SiteInvitableRoleMember},
		siteapi.SiteInvitationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.Contains(t, created.InviteUrl, "/invitations/", "copy-link is returned")
	assert.Equal(t, siteapi.EmailAddress("newbie@acme.test"), created.Resource.Email)

	items, err := c.SiteInvitationsList(ctx, siteapi.SiteInvitationsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	emails := make([]string, 0, len(items))
	for _, inv := range items {
		emails = append(emails, string(inv.Email))
	}
	assert.Contains(t, emails, "newbie@acme.test")
}

func TestSiteInvitationsForbiddenForMember(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)

	_, err := c.SiteInvitationsCreate(context.Background(),
		&siteapi.SiteCreateInvitationInput{Email: "x@acme.test", Role: siteapi.SiteInvitableRoleMember},
		siteapi.SiteInvitationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)
}

func TestSiteInvitationAcceptNewUser(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	// The fixture invite (raw token below) targets an email with no account yet.
	found, err := c.SitePublicInvitationsLookup(ctx, siteapi.SitePublicInvitationsLookupParams{Token: "inv_fixture_token_acme"})
	require.NoError(t, err)
	assert.Equal(t, "Acme", found.WorkspaceName)

	err = c.SitePublicInvitationsAccept(ctx,
		&siteapi.SiteAcceptInvitationInput{Name: siteapi.NewOptUserName("New Bie"), Password: siteapi.NewOptString("s3cret-pass")},
		siteapi.SitePublicInvitationsAcceptParams{Token: "inv_fixture_token_acme"})
	require.NoError(t, err)

	// A verified User now exists and is a member of Acme.
	u, err := env.DB.User.Query().Where(entuser.Email("invited@acme.test")).Only(ctx)
	require.NoError(t, err)
	assert.NotNil(t, u.EmailVerifiedAt, "invite-link acceptance verifies the email")
	m, err := env.DB.Membership.Query().Where(membership.UserID(u.ID), membership.WorkspaceID(fixtures.AcmeID)).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, membership.RoleMember, m.Role)

	// The invite is consumed — no longer pending.
	items, err := c.SiteInvitationsList(ctx, siteapi.SiteInvitationsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	for _, inv := range items {
		assert.NotEqual(t, siteapi.EmailAddress("invited@acme.test"), inv.Email)
	}
}

func TestSiteInvitationAcceptExistingUser(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// An account exists but is not yet a member.
	existing, err := env.DB.User.Query().Where(entuser.Email(fixtures.OutsiderOscarEmail)).Only(ctx)
	require.NoError(t, err)

	// The invitee's own session, no name/password: the account is Verified.
	err = env.SiteActor(t, fixtures.OutsiderOscarEmail).SitePublicInvitationsAccept(ctx, &siteapi.SiteAcceptInvitationInput{},
		siteapi.SitePublicInvitationsAcceptParams{Token: fixtures.InvitationAcmeOscarToken})
	require.NoError(t, err)

	m, err := env.DB.Membership.Query().Where(membership.UserID(existing.ID), membership.WorkspaceID(fixtures.AcmeID)).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, membership.RoleAdmin, m.Role)
}

func TestSiteInvitationExpiredRejected(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	_, err := c.SitePublicInvitationsLookup(ctx, siteapi.SitePublicInvitationsLookupParams{Token: "inv_fixture_token_expired"})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	err = c.SitePublicInvitationsAccept(ctx, &siteapi.SiteAcceptInvitationInput{}, siteapi.SitePublicInvitationsAcceptParams{Token: "inv_fixture_token_expired"})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}

func TestSiteMembershipsList(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	items, err := c.SiteMembershipsList(context.Background(), siteapi.SiteMembershipsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	roles := map[siteapi.EmailAddress]siteapi.SiteMembershipRole{}
	for _, m := range items {
		roles[m.Email] = m.Role
	}
	assert.Equal(t, map[siteapi.EmailAddress]siteapi.SiteMembershipRole{
		fixtures.OwnerJohnEmail:  siteapi.SiteMembershipRoleOwner,
		fixtures.AdminAliceEmail: siteapi.SiteMembershipRoleAdmin,
		fixtures.MemberMaryEmail: siteapi.SiteMembershipRoleMember,
	}, roles)
}

func TestSiteMembershipsLastOwnerGuard(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	// the Acme owner membership is the sole owner (fixture).
	err := c.SiteMembershipsDelete(ctx, siteapi.SiteMembershipsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AcmeOwnerMembershipID)})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

	_, err = c.SiteMembershipsUpdate(ctx,
		&siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleMember},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AcmeOwnerMembershipID)})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
}

func TestSiteMembershipsRoleManagement(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	memberID := int64(fixtures.AcmeMemberMembershipID)

	// A plain member cannot manage roles.
	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	_, err := member.SiteMembershipsUpdate(ctx,
		&siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleAdmin},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(memberID)})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)

	// Owner promotes a member to admin.
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	got, err := owner.SiteMembershipsUpdate(ctx,
		&siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleAdmin},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(memberID)})
	require.NoError(t, err)
	assert.Equal(t, siteapi.SiteMembershipRoleAdmin, got.Role)
}

// An admin manages ordinary members but may not touch an owner: modifying or removing an owner
// needs owners:manage, which only an owner holds (ADR 0061).
func TestSiteMembershipsAdminCannotTouchOwner(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	targetID := int64(fixtures.AcmeMemberMembershipID)

	admin := env.SiteActor(t, fixtures.AdminAliceEmail)

	// Admin can promote an ordinary member.
	_, err := admin.SiteMembershipsUpdate(ctx,
		&siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleAdmin},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(targetID)})
	require.NoError(t, err)

	// Admin cannot demote the owner (the owner membership)...
	_, err = admin.SiteMembershipsUpdate(ctx,
		&siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleMember},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AcmeOwnerMembershipID)})
	demote := testhelper.SiteRefusal(t, err, http.StatusForbidden)
	assert.Equal(t, siteapi.ProblemCodePermissionRequired, demote.Code.Value)

	// ...nor remove them.
	err = admin.SiteMembershipsDelete(ctx, siteapi.SiteMembershipsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AcmeOwnerMembershipID)})
	del := testhelper.SiteRefusal(t, err, http.StatusForbidden)
	assert.Equal(t, siteapi.ProblemCodePermissionRequired, del.Code.Value)
}

func idStr(id int64) siteapi.EntityId {
	return siteapi.EntityId(id)
}
