package site_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/invitation"
	"github.com/getsphericon/sphericon/ent/membership"
	entuser "github.com/getsphericon/sphericon/ent/user"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/signin"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// inviteAdminToAcme invites email to Acme as an Admin and returns the raw link token.
func inviteAdminToAcme(t *testing.T, env *testhelper.TestEnv, email string) string {
	t.Helper()
	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteInvitationsCreate(t.Context(),
		&siteapi.SiteCreateInvitationInput{Email: siteapi.EmailAddress(email), Role: siteapi.SiteInvitableRoleAdmin},
		siteapi.SiteInvitationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	created, ok := res.(*siteapi.SiteCreateInvitationResponse)
	require.Truef(t, ok, "got %T", res)
	return created.InviteUrl[strings.LastIndex(created.InviteUrl, "/")+1:]
}

// signInToAccept is the round trip of a person who follows an Invitation link and chooses the
// provider: `start` carries the link's token.
func signInToAccept(t *testing.T, env *testhelper.TestEnv, fake *testhelper.FakeSignin, b *testhelper.RedirectBrowser, who testhelper.SigninIdentity, token string) testhelper.Reply {
	t.Helper()
	resp := b.Get(t, signinStartPath+"?"+url.Values{"invitation": {token}}.Encode())
	require.Equal(t, http.StatusFound, resp.StatusCode)
	return b.Get(t, fake.Authorize(t, resp.Header.Get("Location"), who))
}

// acmeRoleOf is the role email holds in Acme; the test fails when the User or the Membership is missing.
func acmeRoleOf(t *testing.T, env *testhelper.TestEnv, email string) string {
	t.Helper()
	u, err := env.DB.User.Query().Where(entuser.Email(email)).Only(t.Context())
	require.NoError(t, err)
	m, err := env.DB.Membership.Query().Where(membership.UserID(u.ID), membership.WorkspaceID(fixtures.AcmeID)).Only(t.Context())
	require.NoError(t, err)
	return string(m.Role)
}

func pendingInvitations(t *testing.T, env *testhelper.TestEnv, email string) int {
	t.Helper()
	n, err := env.DB.Invitation.Query().Where(invitation.Email(email), invitation.AcceptedAtIsNil()).Count(t.Context())
	require.NoError(t, err)
	return n
}

func TestSigninAnInvitationIsAcceptedWithTheMatchingProviderEmailWithTheInvitedRole(t *testing.T) {
	env, fake := signinEnv(t)
	token := inviteAdminToAcme(t, env, newcomer.Email)
	b := env.NewRedirectBrowser(t)

	resp := signInToAccept(t, env, fake, b, newcomer, token)

	assert.Equal(t, "/", resp.Header.Get("Location"))
	assert.Equal(t, http.StatusOK, meStatus(t, b))
	assert.Equal(t, "admin", acmeRoleOf(t, env, newcomer.Email), "the Membership has the invited Role")
	assert.Zero(t, pendingInvitations(t, env, newcomer.Email), "the Invitation is accepted")
	u, err := env.DB.User.Query().Where(entuser.Email(newcomer.Email)).Only(t.Context())
	require.NoError(t, err)
	memberships, err := u.QueryMemberships().Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, memberships, "the invited Workspace comes beside the first Workspace every new User gets")
}

func TestSigninAnInvitationMatchesTheAddressWhateverItsCase(t *testing.T) {
	env, fake := signinEnv(t)
	token := inviteAdminToAcme(t, env, "Nina@Gmail.com")

	resp := signInToAccept(t, env, fake, env.NewRedirectBrowser(t), newcomer, token)

	assert.Equal(t, "/", resp.Header.Get("Location"))
	assert.Equal(t, "admin", acmeRoleOf(t, env, newcomer.Email))
}

func TestSigninAnInvitationOpensAClosedRegistrationForItsAddress(t *testing.T) {
	env, fake := signinEnv(t, testhelper.WithConfig(func(c *config.Config) { c.SignupOpen = false }))
	token := inviteAdminToAcme(t, env, newcomer.Email)
	other := testhelper.SigninIdentity{Subject: "google-subject-of-olga", Email: "olga@gmail.com", EmailVerified: true}

	refused := signInWith(t, env, fake, env.NewRedirectBrowser(t), other, "")
	accepted := signInToAccept(t, env, fake, env.NewRedirectBrowser(t), newcomer, token)

	assert.Equal(t, signin.ReasonSignupClosed, refusalCode(t, refused, loginErrorPath), "without an Invitation registration stays closed")
	assert.Equal(t, "/", accepted.Header.Get("Location"))
	assert.Equal(t, "admin", acmeRoleOf(t, env, newcomer.Email))
}

func TestSigninAnInvitationOfAVerifiedUserIsAcceptedThroughTheirProvider(t *testing.T) {
	env, fake := signinEnv(t)
	token := inviteAdminToAcme(t, env, fixtures.OutsiderOscarEmail)
	oscar := testhelper.SigninIdentity{Subject: "google-subject-of-oscar", Email: fixtures.OutsiderOscarEmail, EmailVerified: true, HostedDomain: "nowhere.test"}
	b := env.NewRedirectBrowser(t)

	resp := signInToAccept(t, env, fake, b, oscar, token)

	assert.Equal(t, "/", resp.Header.Get("Location"))
	assert.Equal(t, http.StatusOK, meStatus(t, b))
	assert.Equal(t, "admin", acmeRoleOf(t, env, fixtures.OutsiderOscarEmail))
}

func TestSigninAnInvitationForAnotherAddressIsRefusedAndCreatesNothing(t *testing.T) {
	env, fake := signinEnv(t)
	token := inviteAdminToAcme(t, env, "invited@example.com")
	b := env.NewRedirectBrowser(t)

	resp := signInToAccept(t, env, fake, b, newcomer, token)

	require.Equal(t, http.StatusFound, resp.StatusCode)
	u, err := url.Parse(resp.Header.Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/invitations/"+token, u.Path, "the person lands back on the Invitation")
	assert.Equal(t, string(signin.ReasonInvitationEmailMismatch), u.Query().Get("error"))
	assert.NotEqual(t, http.StatusOK, meStatus(t, b), "no session")
	assert.Zero(t, usersWithEmail(t, env, newcomer.Email), "no User is created")
	assert.Zero(t, usersWithEmail(t, env, "invited@example.com"))
	assert.Equal(t, 1, pendingInvitations(t, env, "invited@example.com"), "the Invitation stays pending")
}

func TestSigninAnInvitationIsRefusedForALinkedIdentityOfAnotherAccount(t *testing.T) {
	env, fake := signinEnv(t)
	token := inviteAdminToAcme(t, env, fixtures.OutsiderOscarEmail)
	mary := testhelper.SigninIdentity{Subject: fixtures.MaryGoogleIdentitySubject, Email: fixtures.OutsiderOscarEmail, EmailVerified: true, HostedDomain: "nowhere.test"}
	b := env.NewRedirectBrowser(t)

	resp := signInToAccept(t, env, fake, b, mary, token)

	require.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Location"), "error="+string(signin.ReasonInvitationEmailMismatch))
	assert.NotEqual(t, http.StatusOK, meStatus(t, b), "Mary's account is not Oscar's")
	joined, err := env.DB.Membership.Query().Where(membership.UserID(fixtures.OutsiderOscarID), membership.WorkspaceID(fixtures.AcmeID)).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, joined, "Oscar did not join Acme")
	assert.Equal(t, 1, pendingInvitations(t, env, fixtures.OutsiderOscarEmail))
}

func TestSigninAnUnprovenEmailCannotAcceptAnInvitation(t *testing.T) {
	env, fake := signinEnv(t)
	token := inviteAdminToAcme(t, env, newcomer.Email)
	unverified := newcomer
	unverified.EmailVerified = false

	resp := signInToAccept(t, env, fake, env.NewRedirectBrowser(t), unverified, token)

	assert.Contains(t, resp.Header.Get("Location"), "error=email_unverified")
	assert.Zero(t, usersWithEmail(t, env, newcomer.Email))
	assert.Equal(t, 1, pendingInvitations(t, env, newcomer.Email))
}

func TestSigninAnInvitationThatIsNotPendingIsRefusedAndCreatesNothing(t *testing.T) {
	env, fake := signinEnv(t)
	b := env.NewRedirectBrowser(t)

	resp := signInToAccept(t, env, fake, b, newcomer, "not-a-token")

	require.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Location"), "error=invitation_invalid")
	assert.Zero(t, usersWithEmail(t, env, newcomer.Email))
}
