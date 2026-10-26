package site_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/user"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func acceptInvitation(t *testing.T, c *siteapi.Client, token string, in *siteapi.SiteAcceptInvitationInput) siteapi.SitePublicInvitationsAcceptRes {
	t.Helper()
	res, err := c.SitePublicInvitationsAccept(t.Context(), in, siteapi.SitePublicInvitationsAcceptParams{Token: token})
	require.NoError(t, err)
	return res
}

func isMemberOfAcme(t *testing.T, env *testhelper.TestEnv, email string) bool {
	t.Helper()
	u, err := env.DB.User.Query().Where(user.Email(email)).Only(t.Context())
	require.NoError(t, err)
	ok, err := env.DB.Membership.Query().
		Where(membership.UserID(u.ID), membership.WorkspaceID(fixtures.AcmeID)).Exist(t.Context())
	require.NoError(t, err)
	return ok
}

func TestAnInvitationForAVerifiedUserNeedsTheirSession(t *testing.T) {
	env := testhelper.Setup(t)
	token := inviteToAcme(t, env, fixtures.OutsiderOscarEmail)

	res := acceptInvitation(t, env.SiteAnonymous(t), token, &siteapi.SiteAcceptInvitationInput{})
	assert.IsType(t, &siteapi.SitePublicInvitationsAcceptUnauthorized{}, res)
	assert.False(t, isMemberOfAcme(t, env, fixtures.OutsiderOscarEmail))
}

func TestAnInvitationForAVerifiedUserRefusesAnotherUsersSession(t *testing.T) {
	env := testhelper.Setup(t)
	token := inviteToAcme(t, env, fixtures.OutsiderOscarEmail)

	res := acceptInvitation(t, env.SiteActor(t, fixtures.OwnerJaneEmail), token, &siteapi.SiteAcceptInvitationInput{})
	assert.IsType(t, &siteapi.SitePublicInvitationsAcceptForbidden{}, res)
	assert.False(t, isMemberOfAcme(t, env, fixtures.OutsiderOscarEmail))
	assert.False(t, isMemberOfAcme(t, env, fixtures.OwnerJaneEmail))
}

func TestAnInvitationForAVerifiedUserAcceptsTheirSessionAndReadsNoPassword(t *testing.T) {
	env := testhelper.Setup(t)
	token := inviteToAcme(t, env, fixtures.OutsiderOscarEmail)
	before, err := env.DB.User.Query().Where(user.Email(fixtures.OutsiderOscarEmail)).Only(t.Context())
	require.NoError(t, err)

	res := acceptInvitation(t, env.SiteActor(t, fixtures.OutsiderOscarEmail), token, &siteapi.SiteAcceptInvitationInput{
		Name: siteapi.NewOptUserName("Hijacker"), Password: siteapi.NewOptString("another-password-1"),
	})
	require.IsType(t, &siteapi.SitePublicInvitationsAcceptOK{}, res)
	assert.True(t, isMemberOfAcme(t, env, fixtures.OutsiderOscarEmail))

	after, err := env.DB.User.Query().Where(user.Email(fixtures.OutsiderOscarEmail)).Only(t.Context())
	require.NoError(t, err)
	assert.Equal(t, before.PasswordHash, after.PasswordHash)
	assert.Equal(t, before.Name, after.Name)
}

func TestAnInvitationForAnUnverifiedAddressNeedsNoSession(t *testing.T) {
	env := testhelper.Setup(t)
	register(t, env, "Squatter", "victim@example.com")
	token := inviteToAcme(t, env, "victim@example.com")

	// A signed-in stranger does not get in the way of the invitee's own accept.
	res := acceptInvitation(t, env.SiteActor(t, fixtures.OwnerJaneEmail), token, &siteapi.SiteAcceptInvitationInput{
		Name: siteapi.NewOptUserName("Victim"), Password: siteapi.NewOptString("victim-password-1"),
	})
	assert.IsType(t, &siteapi.SitePublicInvitationsAcceptOK{}, res)
	assert.True(t, isMemberOfAcme(t, env, "victim@example.com"))
}

func TestAStaleSessionDoesNotBlockAnInvitationForAnUnverifiedAddress(t *testing.T) {
	env := testhelper.Setup(t)
	token := inviteToAcme(t, env, "newcomer@example.com")

	res := acceptInvitation(t, env.SiteWithToken(t, "stale-garbage"), token, &siteapi.SiteAcceptInvitationInput{
		Name: siteapi.NewOptUserName("Newcomer"), Password: siteapi.NewOptString("newcomer-password-1"),
	})
	assert.IsType(t, &siteapi.SitePublicInvitationsAcceptOK{}, res)
}
