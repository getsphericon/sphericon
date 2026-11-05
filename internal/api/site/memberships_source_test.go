package site_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func membersOf(t *testing.T, env *testhelper.TestEnv, asEmail string, workspace siteapi.WorkspaceId) []siteapi.SiteMembershipResource {
	t.Helper()
	list, err := env.SiteActor(t, asEmail).SiteMembershipsList(t.Context(), siteapi.SiteMembershipsListParams{WorkspaceId: workspace})
	require.NoError(t, err)
	return list
}

func memberWithEmail(t *testing.T, members []siteapi.SiteMembershipResource, email string) siteapi.SiteMembershipResource {
	t.Helper()
	m, found := lo.Find(members, func(m siteapi.SiteMembershipResource) bool { return string(m.Email) == email })
	require.Truef(t, found, "%s is not listed", email)
	return m
}

func TestAMembershipFromBeforeSourceWasRecordedHasNone(t *testing.T) {
	env := testhelper.Setup(t)

	members := membersOf(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID)
	john := memberWithEmail(t, members, fixtures.OwnerJohnEmail)

	assert.False(t, john.Source.Set)
}

func TestAWorkspaceCreatorIsRecordedAsTheCreator(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteWorkspacesCreate(t.Context(), &siteapi.SiteCreateWorkspaceInput{Name: "Second Brand"})
	require.NoError(t, err)
	created := res

	members := membersOf(t, env, fixtures.OwnerJohnEmail, created.ID)
	john := memberWithEmail(t, members, fixtures.OwnerJohnEmail)
	assert.Equal(t, siteapi.NewOptSiteMembershipSource(siteapi.SiteMembershipSourceCreator), john.Source)
}

func TestAnAcceptedInvitationIsRecordedAsTheMembershipsSource(t *testing.T) {
	env := testhelper.Setup(t)
	token := fixtures.InvitationAcmeOscarToken

	require.NoError(t, acceptInvitation(t, env.SiteActor(t, fixtures.OutsiderOscarEmail), token, &siteapi.SiteAcceptInvitationInput{}))

	members := membersOf(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID)
	oscar := memberWithEmail(t, members, fixtures.OutsiderOscarEmail)
	assert.Equal(t, siteapi.NewOptSiteMembershipSource(siteapi.SiteMembershipSourceInvitation), oscar.Source)
}

func TestAnAdminInvitationRaisesAnExistingMemberToAdmin(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, acceptInvitation(t, env.SiteActor(t, fixtures.MemberMaryEmail), "inv_fixture_token_acme_mary_admin", &siteapi.SiteAcceptInvitationInput{}))

	members := membersOf(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID)
	mary := memberWithEmail(t, members, fixtures.MemberMaryEmail)
	assert.Equal(t, siteapi.SiteMembershipRoleAdmin, mary.Role)
	assert.False(t, mary.Source.Set, "a raised Role keeps how the person got in")
}

func TestAnInvitationNeverLowersAnAdmin(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, acceptInvitation(t, env.SiteActor(t, fixtures.AdminAliceEmail), "inv_fixture_token_acme_alice_member", &siteapi.SiteAcceptInvitationInput{}))

	members := membersOf(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID)
	alice := memberWithEmail(t, members, fixtures.AdminAliceEmail)
	assert.Equal(t, siteapi.SiteMembershipRoleAdmin, alice.Role)
}

func TestAnInvitationNeverLowersAnOwner(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, acceptInvitation(t, env.SiteActor(t, fixtures.OwnerJohnEmail), "inv_fixture_token_acme_john_admin", &siteapi.SiteAcceptInvitationInput{}))

	members := membersOf(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID)
	john := memberWithEmail(t, members, fixtures.OwnerJohnEmail)
	assert.Equal(t, siteapi.SiteMembershipRoleOwner, john.Role)
}
