package site_test

import (
	"context"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/ent/membership"
	entuser "github.com/getsphericon/sphericon/ent/user"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteInvitationsCreateAndList(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail) // owner
	ctx := context.Background()

	res, err := c.SiteInvitationsCreate(ctx,
		&siteapi.SiteCreateInvitationInput{Email: "newbie@acme.test", Role: siteapi.SiteInvitableRoleMember},
		siteapi.SiteInvitationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	created, ok := res.(*siteapi.SiteCreateInvitationResponse)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, created.InviteUrl, "/invitations/", "copy-link is returned")
	assert.Equal(t, siteapi.EmailAddress("newbie@acme.test"), created.Resource.Email)

	list, err := c.SiteInvitationsList(ctx, siteapi.SiteInvitationsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	items, ok := list.(*siteapi.SiteInvitationsListOKApplicationJSON)
	require.Truef(t, ok, "got %T", list)
	emails := make([]string, 0, len(*items))
	for _, inv := range *items {
		emails = append(emails, string(inv.Email))
	}
	assert.Contains(t, emails, "newbie@acme.test")
}

func TestSiteInvitationsForbiddenForMember(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)

	res, err := c.SiteInvitationsCreate(context.Background(),
		&siteapi.SiteCreateInvitationInput{Email: "x@acme.test", Role: siteapi.SiteInvitableRoleMember},
		siteapi.SiteInvitationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteInvitationsCreateForbidden{}, res)
}

func TestSiteInvitationAcceptNewUser(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	// The fixture invite (raw token below) targets an email with no account yet.
	lookup, err := c.SitePublicInvitationsLookup(ctx, siteapi.SitePublicInvitationsLookupParams{Token: "inv_fixture_token_acme"})
	require.NoError(t, err)
	found, ok := lookup.(*siteapi.SiteInvitationLookupResult)
	require.Truef(t, ok, "got %T", lookup)
	assert.Equal(t, "Acme", found.WorkspaceName)

	accept, err := c.SitePublicInvitationsAccept(ctx,
		&siteapi.SiteAcceptInvitationInput{Name: siteapi.NewOptString("New Bie"), Password: siteapi.NewOptString("s3cret-pass")},
		siteapi.SitePublicInvitationsAcceptParams{Token: "inv_fixture_token_acme"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SitePublicInvitationsAcceptOK{}, accept)

	// A verified User now exists and is a member of Acme.
	u, err := env.DB.User.Query().Where(entuser.Email("invited@acme.test")).Only(ctx)
	require.NoError(t, err)
	assert.NotNil(t, u.EmailVerifiedAt, "invite-link acceptance verifies the email")
	m, err := env.DB.Membership.Query().Where(membership.UserID(u.ID), membership.WorkspaceID(fixtures.AcmeID)).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, membership.RoleMember, m.Role)

	// The invite is consumed — no longer pending.
	list, err := c.SiteInvitationsList(ctx, siteapi.SiteInvitationsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	items := list.(*siteapi.SiteInvitationsListOKApplicationJSON)
	for _, inv := range *items {
		assert.NotEqual(t, siteapi.EmailAddress("invited@acme.test"), inv.Email)
	}
}

func TestSiteInvitationAcceptExistingUser(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	// An account exists but is not yet a member.
	existing, err := env.DB.User.Query().Where(entuser.Email(fixtures.OutsiderOscarEmail)).Only(ctx)
	require.NoError(t, err)

	res, err := c.SiteInvitationsCreate(ctx,
		&siteapi.SiteCreateInvitationInput{Email: fixtures.OutsiderOscarEmail, Role: siteapi.SiteInvitableRoleAdmin},
		siteapi.SiteInvitationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	created := res.(*siteapi.SiteCreateInvitationResponse)
	token := created.InviteUrl[strings.LastIndex(created.InviteUrl, "/")+1:]

	// The invitee's own session, no name/password: the account is Verified.
	accept, err := env.SiteActor(t, fixtures.OutsiderOscarEmail).SitePublicInvitationsAccept(ctx, &siteapi.SiteAcceptInvitationInput{}, siteapi.SitePublicInvitationsAcceptParams{Token: token})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SitePublicInvitationsAcceptOK{}, accept)

	m, err := env.DB.Membership.Query().Where(membership.UserID(existing.ID), membership.WorkspaceID(fixtures.AcmeID)).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, membership.RoleAdmin, m.Role)
}

func TestSiteInvitationExpiredRejected(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	lookup, err := c.SitePublicInvitationsLookup(ctx, siteapi.SitePublicInvitationsLookupParams{Token: "inv_fixture_token_expired"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.ProblemDetails{}, lookup)

	accept, err := c.SitePublicInvitationsAccept(ctx, &siteapi.SiteAcceptInvitationInput{}, siteapi.SitePublicInvitationsAcceptParams{Token: "inv_fixture_token_expired"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SitePublicInvitationsAcceptNotFound{}, accept)
}

func TestSiteMembershipsList(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	list, err := c.SiteMembershipsList(context.Background(), siteapi.SiteMembershipsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	items, ok := list.(*siteapi.SiteMembershipsListOKApplicationJSON)
	require.Truef(t, ok, "got %T", list)
	roles := map[siteapi.EmailAddress]siteapi.SiteMembershipRole{}
	for _, m := range *items {
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
	del, err := c.SiteMembershipsDelete(ctx, siteapi.SiteMembershipsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AcmeOwnerMembershipID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteMembershipsDeleteUnprocessableEntity{}, del)

	upd, err := c.SiteMembershipsUpdate(ctx,
		&siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleMember},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AcmeOwnerMembershipID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteMembershipsUpdateUnprocessableEntity{}, upd)
}

func TestSiteMembershipsRoleManagement(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	memberID := int64(fixtures.AcmeMemberMembershipID)

	// A plain member cannot manage roles.
	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	forbidden, err := member.SiteMembershipsUpdate(ctx,
		&siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleAdmin},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(memberID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteMembershipsUpdateForbidden{}, forbidden)

	// Owner promotes a member to admin.
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	upd, err := owner.SiteMembershipsUpdate(ctx,
		&siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleAdmin},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(memberID)})
	require.NoError(t, err)
	got, ok := upd.(*siteapi.SiteMembershipResource)
	require.Truef(t, ok, "got %T", upd)
	assert.Equal(t, siteapi.SiteMembershipRoleAdmin, got.Role)
}

// An admin manages ordinary members but may not touch an owner — only an owner
// can modify or remove another owner.
func TestSiteMembershipsAdminCannotTouchOwner(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	targetID := int64(fixtures.AcmeMemberMembershipID)

	admin := env.SiteActor(t, fixtures.AdminAliceEmail)

	// Admin can promote an ordinary member.
	upd, err := admin.SiteMembershipsUpdate(ctx,
		&siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleAdmin},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(targetID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteMembershipResource{}, upd)

	// Admin cannot demote the owner (the owner membership)...
	demote, err := admin.SiteMembershipsUpdate(ctx,
		&siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleMember},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AcmeOwnerMembershipID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteMembershipsUpdateForbidden{}, demote)

	// ...nor remove them.
	del, err := admin.SiteMembershipsDelete(ctx, siteapi.SiteMembershipsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AcmeOwnerMembershipID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteMembershipsDeleteForbidden{}, del)
}

func idStr(id int64) siteapi.EntityId {
	return siteapi.EntityId(id)
}
