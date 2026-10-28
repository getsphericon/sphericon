package site_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func surveySigninEnv(t *testing.T) (*testhelper.TestEnv, *testhelper.FakeSignin) {
	t.Helper()
	return signinEnv(t)
}

func TestSigninANewPersonOwesTheSurveyWhenTheInstanceAsksIt(t *testing.T) {
	env, fake := surveySigninEnv(t)

	signInWith(t, fake, env.NewRedirectBrowser(t), newcomer, "")

	assert.True(t, surveyDue(t, env.SiteActor(t, newcomer.Email)))
}

func TestSigninANewPersonIsNotAskedWhenTheSurveyIsOff(t *testing.T) {
	env, fake := signinEnv(t, testhelper.WithMode(config.ModeSelfHosted))

	signInWith(t, fake, env.NewRedirectBrowser(t), newcomer, "")

	assert.False(t, surveyDue(t, env.SiteActor(t, newcomer.Email)))
}

func TestSigninTheSecondSignInOfTheSamePersonKeepsTheSurveyDue(t *testing.T) {
	env, fake := surveySigninEnv(t)
	signInWith(t, fake, env.NewRedirectBrowser(t), newcomer, "")

	signInWith(t, fake, env.NewRedirectBrowser(t), newcomer, "")

	assert.True(t, surveyDue(t, env.SiteActor(t, newcomer.Email)))
}

func TestSigninANewPersonWhoAcceptsAnInvitationIsNeverAsked(t *testing.T) {
	env, fake := surveySigninEnv(t)
	token := inviteAdminToAcme(t, env, newcomer.Email)

	signInToAccept(t, fake, env.NewRedirectBrowser(t), newcomer, token)

	assert.False(t, surveyDue(t, env.SiteActor(t, newcomer.Email)))
}

func TestSigninLinkingAProviderToAnExistingUserDoesNotAskTheSurvey(t *testing.T) {
	env, fake := surveySigninEnv(t)
	oscar := testhelper.SigninIdentity{Subject: "google-subject-of-oscar", Email: fixtures.OutsiderOscarEmail, EmailVerified: true, HostedDomain: "nowhere.test"}

	signInWith(t, fake, env.NewRedirectBrowser(t), oscar, "")

	assert.False(t, surveyDue(t, env.SiteActor(t, fixtures.OutsiderOscarEmail)))
}

func TestSigninANewPersonAdmittedThroughAJoinDomainIsNeverAsked(t *testing.T) {
	env, fake := surveySigninEnv(t)
	enableAcmeJoin(t, env)
	colleague := testhelper.SigninIdentity{Subject: "google-subject-of-colleague", Email: "colleague@acme.com", EmailVerified: true, HostedDomain: "acme.com"}

	signInWith(t, fake, env.NewRedirectBrowser(t), colleague, "")

	assert.False(t, surveyDue(t, env.SiteActor(t, colleague.Email)))
}

func TestAPasswordSignUpAdmittedThroughAJoinDomainIsNeverAsked(t *testing.T) {
	env := testhelper.Setup(t)
	enableAcmeJoin(t, env)

	joiner := signUp(t, env, "newbie@acme.com")

	assert.False(t, surveyDue(t, joiner.client))
}

func TestAPasswordSignUpOnAJoinDomainWithTheSwitchOffStaysDue(t *testing.T) {
	env := testhelper.Setup(t)

	outsider := signUp(t, env, "newbie@acme.com")

	assert.True(t, surveyDue(t, outsider.client))
}
