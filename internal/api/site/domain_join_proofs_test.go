package site_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// colleague is a person whose provider vouches for an address on Acme's verified Join domain.
var colleague = testhelper.SigninIdentity{Subject: "771204", Login: "colleague", Email: "colleague@acme.com", EmailVerified: true, Name: "Colleague"}

func verificationOff(c *config.Config) { c.RequireEmailVerification = false }

// requireDomainJoined asserts that email is a `member` of Acme with source `domain` and that
// the one `membership.domain_join` entry has the system as its actor.
func requireDomainJoined(t *testing.T, env *testhelper.TestEnv, email string) {
	t.Helper()
	joined := acmeMembersNamed(acmeMembers(t, env.SiteActor(t, fixtures.OwnerJohnEmail)), email)
	require.Len(t, joined, 1, "%s is a member of Acme", email)
	assert.Equal(t, siteapi.SiteMembershipRoleMember, joined[0].Role)
	assert.Equal(t, siteapi.NewOptSiteMembershipSource(siteapi.SiteMembershipSourceDomain), joined[0].Source)
	got := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionMembershipDomainJoin)
	require.Len(t, got, 1)
	assert.Equal(t, "system", string(got[0].Actor.Kind))
}

func requireNotInAcme(t *testing.T, env *testhelper.TestEnv, email string) {
	t.Helper()
	assert.Empty(t, acmeMembersNamed(acmeMembers(t, env.SiteActor(t, fixtures.OwnerJohnEmail)), email))
}

func TestAProviderSigninThatCreatesAUserJoinsTheWorkspaceOfTheAddressDomain(t *testing.T) {
	env, fake := githubEnv(t)
	enableAcmeJoin(t, env)

	signInWithGitHub(t, fake, env.NewRedirectBrowser(t), colleague)

	requireDomainJoined(t, env, colleague.Email)
}

func TestAProviderSigninOfAKnownIdentityJoinsNobodyAgain(t *testing.T) {
	env, fake := githubEnv(t)
	signInWithGitHub(t, fake, env.NewRedirectBrowser(t), colleague)
	enableAcmeJoin(t, env)

	signInWithGitHub(t, fake, env.NewRedirectBrowser(t), colleague)

	requireNotInAcme(t, env, colleague.Email)
}

// ADR 0063: sign-up is always open, so the Mode does not stop a new User from joining by domain.
func TestAProviderSigninThatCreatesAUserOnASelfHostedInstanceJoinsTheWorkspaceOfTheAddressDomain(t *testing.T) {
	env, fake := githubEnv(t, testhelper.WithMode(config.ModeSelfHosted))
	enableAcmeJoin(t, env)

	signInWithGitHub(t, fake, env.NewRedirectBrowser(t), colleague)

	requireDomainJoined(t, env, colleague.Email)
}

func TestAProviderLinkOfAnAddressNeverProvenJoinsTheUser(t *testing.T) {
	env, fake := githubEnv(t, testhelper.WithConfig(verificationOff))
	_, err := env.SiteAnonymous(t).SiteAuthSignUp(t.Context(), &siteapi.SiteSignUpInput{
		Name: "Colleague", Email: siteapi.EmailAddress(colleague.Email), Password: siteapi.NewOptString(joinerPassword),
	})
	require.NoError(t, err)
	enableAcmeJoin(t, env)

	signInWithGitHub(t, fake, env.NewRedirectBrowser(t), colleague)

	requireDomainJoined(t, env, colleague.Email)
}

func TestAProviderLinkOfAnAlreadyProvenAddressJoinsNobody(t *testing.T) {
	env, fake := githubEnv(t)
	signUpAndVerify(t, env, "Colleague", colleague.Email, joinerPassword)
	enableAcmeJoin(t, env)

	signInWithGitHub(t, fake, env.NewRedirectBrowser(t), colleague)

	requireNotInAcme(t, env, colleague.Email)
}

func TestAProviderTakeOverOfAPendingSignUpJoinsTheUser(t *testing.T) {
	env, fake := githubEnv(t)
	signUpPending(t, env, "Squatter", colleague.Email, joinerPassword)
	enableAcmeJoin(t, env)

	signInWithGitHub(t, fake, env.NewRedirectBrowser(t), colleague)

	requireDomainJoined(t, env, colleague.Email)
}

// changeEmail runs the whole confirmed email change of the person's session.
func changeEmail(t *testing.T, env *testhelper.TestEnv, c *siteapi.Client, to string) {
	t.Helper()
	err := c.SiteUserEmailChange(t.Context(), &siteapi.SiteEmailChangeInput{NewEmail: siteapi.EmailAddress(to), CurrentPassword: joinerPassword})
	require.NoError(t, err)
	_, body := lastSystemEmail(t, env)
	_, err = env.SiteAnonymous(t).SiteAuthConfirmEmailChange(t.Context(),
		&siteapi.SiteConfirmEmailChangeInput{Token: tokenFromEmail(t, body)}, siteapi.SiteAuthConfirmEmailChangeParams{})
	require.NoError(t, err)
}

func TestAConfirmedEmailChangeToTheDomainJoinsTheUser(t *testing.T) {
	env := testhelper.Setup(t)
	enableAcmeJoin(t, env)
	person := signUp(t, env, "person@elsewhere.example")

	changeEmail(t, env, person.client, colleague.Email)

	requireDomainJoined(t, env, colleague.Email)
}

func TestAConfirmedEmailChangeBeforeTheSwitchIsOnJoinsNobody(t *testing.T) {
	env := testhelper.Setup(t)
	person := signUp(t, env, "person@elsewhere.example")

	changeEmail(t, env, person.client, colleague.Email)
	enableAcmeJoin(t, env)

	requireNotInAcme(t, env, colleague.Email)
}

func TestARemovedMemberRejoinsByConfirmingAnotherAddressOnTheDomainButNotByLoggingIn(t *testing.T) {
	env := testhelper.Setup(t)
	enableAcmeJoin(t, env)
	person := signUp(t, env, colleague.Email)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	joined := acmeMembersNamed(acmeMembers(t, owner), colleague.Email)
	require.Len(t, joined, 1)
	err := owner.SiteMembershipsDelete(t.Context(), siteapi.SiteMembershipsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: joined[0].ID})
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, loginStatus(t, env, colleague.Email, joinerPassword))
	requireNotInAcme(t, env, colleague.Email)

	changeEmail(t, env, person.client, "again@acme.com")
	assert.Len(t, acmeMembersNamed(acmeMembers(t, owner), "again@acme.com"), 1, "the accepted trade-off: a new proof rejoins")
}

func acceptAnonymously(t *testing.T, env *testhelper.TestEnv, token string) {
	t.Helper()
	err := env.SiteAnonymous(t).SitePublicInvitationsAccept(t.Context(),
		&siteapi.SiteAcceptInvitationInput{Name: siteapi.NewOptUserName("Colleague"), Password: siteapi.NewOptString(joinerPassword)},
		siteapi.SitePublicInvitationsAcceptParams{Token: token})
	require.NoError(t, err)
}

func TestAnInvitationToAnotherWorkspaceThatProvesANewAddressAlsoJoinsTheDomainWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	enableAcmeJoin(t, env)
	token := fixtures.InvitationGlobexColleagueToken

	acceptAnonymously(t, env, token)

	requireDomainJoined(t, env, colleague.Email)
}

func TestAnInvitationForAnAlreadyProvenAddressTriggersNoDomainJoin(t *testing.T) {
	env := testhelper.Setup(t)
	person := signUp(t, env, colleague.Email)
	enableAcmeJoin(t, env)
	token := fixtures.InvitationGlobexColleagueToken

	err := person.client.SitePublicInvitationsAccept(t.Context(), &siteapi.SiteAcceptInvitationInput{},
		siteapi.SitePublicInvitationsAcceptParams{Token: token})
	require.NoError(t, err)

	requireNotInAcme(t, env, colleague.Email)
}

func TestAnAdminInvitationToTheDomainWorkspaceKeepsTheAdminRoleOfAFirstProof(t *testing.T) {
	env := testhelper.Setup(t)
	enableAcmeJoin(t, env)
	token := fixtures.InvitationAcmeColleagueToken

	acceptAnonymously(t, env, token)

	got := acmeMembersNamed(acmeMembers(t, env.SiteActor(t, fixtures.OwnerJohnEmail)), colleague.Email)
	require.Len(t, got, 1)
	assert.Equal(t, siteapi.SiteMembershipRoleAdmin, got[0].Role, "the domain rule never lowers an explicit Role")
	assert.Empty(t, entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionMembershipDomainJoin))
}

func TestAnAdminInvitationRaisesADomainJoinedMember(t *testing.T) {
	env := testhelper.Setup(t)
	token := fixtures.InvitationAcmeColleagueToken
	enableAcmeJoin(t, env)
	person := signUp(t, env, colleague.Email)
	requireDomainJoined(t, env, colleague.Email)

	err := person.client.SitePublicInvitationsAccept(t.Context(), &siteapi.SiteAcceptInvitationInput{},
		siteapi.SitePublicInvitationsAcceptParams{Token: token})
	require.NoError(t, err)

	got := acmeMembersNamed(acmeMembers(t, env.SiteActor(t, fixtures.OwnerJohnEmail)), colleague.Email)
	require.Len(t, got, 1)
	assert.Equal(t, siteapi.SiteMembershipRoleAdmin, got[0].Role)
}

func resetPassword(t *testing.T, env *testhelper.TestEnv, email string) {
	t.Helper()
	c := env.SiteAnonymous(t)
	err := c.SiteAuthForgotPassword(t.Context(), &siteapi.SiteForgotPasswordInput{Email: siteapi.EmailAddress(email)})
	require.NoError(t, err)
	_, body := lastSystemEmail(t, env)
	err = c.SiteAuthResetPassword(t.Context(), &siteapi.SiteResetPasswordInput{Token: tokenFromEmail(t, body), Password: "brandnewpass1"})
	require.NoError(t, err)
}

func TestAResetLinkThatProvesAnAddressNeverProvenJoinsTheUser(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(verificationOff))
	_, err := env.SiteAnonymous(t).SiteAuthSignUp(t.Context(), &siteapi.SiteSignUpInput{
		Name: "Colleague", Email: siteapi.EmailAddress(colleague.Email), Password: siteapi.NewOptString(joinerPassword),
	})
	require.NoError(t, err)
	enableAcmeJoin(t, env)

	resetPassword(t, env, colleague.Email)

	requireDomainJoined(t, env, colleague.Email)
}

func TestAResetLinkOfAnAlreadyProvenAddressJoinsNobody(t *testing.T) {
	env := testhelper.Setup(t)
	signUpAndVerify(t, env, "Colleague", colleague.Email, joinerPassword)
	enableAcmeJoin(t, env)

	resetPassword(t, env, colleague.Email)

	requireNotInAcme(t, env, colleague.Email)
}
