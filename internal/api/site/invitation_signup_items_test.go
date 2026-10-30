package site_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/consentrecord"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Invitation that sets up an account asks the Sign-up items as the sign-up form does (ADR
// 0062); one accepted by an existing User asks nothing, since consent follows the User.

const inviteeAddress = "newcomer@example.com"

func setUpAccountAnswering(t *testing.T, env *testhelper.TestEnv, token string, answers ...siteapi.SiteSignupItemAnswer) siteapi.SitePublicInvitationsAcceptRes {
	t.Helper()
	return acceptInvitation(t, env.SiteAnonymous(t), token, &siteapi.SiteAcceptInvitationInput{
		Name: siteapi.NewOptString("Newcomer"), Password: siteapi.NewOptString(formPassword), SignupItems: answers,
	})
}

func TestAnInvitationForANewAccountWithoutARequiredAcceptanceIsRefusedWithAFieldNamingIt(t *testing.T) {
	env := testhelper.Setup(t)
	token := inviteToAcme(t, env, inviteeAddress)
	withSignupItems(t, env)

	res := setUpAccountAnswering(t, env, token, newsUnticked)

	problem, ok := res.(*siteapi.SitePublicInvitationsAcceptUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.ProblemDetailsErrors{
		"signupItems." + fixtures.ServiceTermsKey: {"accept the Terms of service to sign up"},
	}, problem.Errors.Value)
	assert.False(t, userExists(t, env, inviteeAddress))
	lookup, err := env.SiteAnonymous(t).SitePublicInvitationsLookup(t.Context(), siteapi.SitePublicInvitationsLookupParams{Token: token})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteInvitationLookupResult{}, lookup, "the Invitation is still pending")
}

func TestAnInvitationForANewAccountWithTheAnswersCreatesTheMemberAndRecordsThemAsMadeOnTheInvitation(t *testing.T) {
	env := testhelper.Setup(t)
	token := inviteToAcme(t, env, inviteeAddress)
	withSignupItems(t, env)

	res := setUpAccountAnswering(t, env, token, termsTicked, newsUnticked)

	require.IsType(t, &siteapi.SitePublicInvitationsAcceptOK{}, res)
	assert.True(t, isMemberOfAcme(t, env, inviteeAddress))
	newcomer := userByEmail(t, env, inviteeAddress)
	assert.Equal(t, []string{
		fixtures.ServiceTermsKey + ": accepted",
		fixtures.OperatorNewsKey + ": declined",
		fixtures.DataNoticeKey + ": shown",
	}, currentAnswers(t, env, newcomer.ID))
	assert.Equal(t, []consentrecord.Source{
		consentrecord.SourceInvitation, consentrecord.SourceInvitation, consentrecord.SourceInvitation,
	}, answerSources(t, env, newcomer.ID))
}

func TestAnInvitationForANewAccountAnsweringAStaleVersionIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	token := inviteToAcme(t, env, inviteeAddress)
	withSignupItems(t, env)

	res := setUpAccountAnswering(t, env, token, itemAnswer(fixtures.ServiceTermsKey, fixtures.ServiceTermsStaleLabel, true))

	problem, ok := res.(*siteapi.SitePublicInvitationsAcceptUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, []string{"signupItems"}, lo.Keys(problem.Errors.Value))
	assert.False(t, userExists(t, env, inviteeAddress))
}

// The Invitation completes an Unverified User as it sets up a new one, so it asks the same.
func TestAnInvitationThatCompletesAnUnverifiedUserAsksTheSignupItems(t *testing.T) {
	env := testhelper.Setup(t)
	signUpLink(t, env, "Squatter", inviteeAddress)
	token := inviteToAcme(t, env, inviteeAddress)
	withSignupItems(t, env)

	refused := setUpAccountAnswering(t, env, token)
	accepted := setUpAccountAnswering(t, env, token, termsTicked)

	assert.IsType(t, &siteapi.SitePublicInvitationsAcceptUnprocessableEntity{}, refused)
	require.IsType(t, &siteapi.SitePublicInvitationsAcceptOK{}, accepted)
	assert.True(t, isMemberOfAcme(t, env, inviteeAddress))
	assert.Equal(t, []string{
		fixtures.ServiceTermsKey + ": accepted",
		fixtures.DataNoticeKey + ": shown",
	}, currentAnswers(t, env, userByEmail(t, env, inviteeAddress).ID))
}

// An existing User who holds the current versions is asked nothing; one who does not is stopped by
// the acceptance gate first (TestAGatedUserCannotAcceptAnInvitation).
func TestAnExistingUserAcceptsAnInvitationWithoutBeingAskedTheSignupItems(t *testing.T) {
	env := testhelper.Setup(t)
	token := inviteToAcme(t, env, fixtures.OutsiderOscarEmail)
	withSignupItems(t, env)
	oscar := env.SiteActor(t, fixtures.OutsiderOscarEmail)
	acceptPending(t, oscar, termsAccepted)
	before := currentAnswers(t, env, fixtures.OutsiderOscarID)

	res := acceptInvitation(t, oscar, token, &siteapi.SiteAcceptInvitationInput{})

	require.IsType(t, &siteapi.SitePublicInvitationsAcceptOK{}, res)
	assert.True(t, isMemberOfAcme(t, env, fixtures.OutsiderOscarEmail))
	after := currentAnswers(t, env, fixtures.OutsiderOscarID)
	assert.Equal(t, before, after)
}

func TestAJoinDomainAdmissionAsksNothing(t *testing.T) {
	env := testhelper.Setup(t)
	person := signUp(t, env, "person@elsewhere.example")
	enableAcmeJoin(t, env)
	withSignupItems(t, env)
	acceptPending(t, person.client, termsAccepted)
	acceptTermsAs(t, env, fixtures.OwnerJohnEmail)
	before := currentAnswers(t, env, userByEmail(t, env, "person@elsewhere.example").ID)

	changeEmail(t, env, person.client, colleague.Email)

	requireDomainJoined(t, env, colleague.Email)
	after := currentAnswers(t, env, userByEmail(t, env, colleague.Email).ID)
	assert.Equal(t, before, after)
}

// answerSources is where each of the User's current answers was made.
func answerSources(t *testing.T, env *testhelper.TestEnv, userID int64) []consentrecord.Source {
	t.Helper()
	records, err := env.ConsentRecords.Current(t.Context(), userID)
	require.NoError(t, err)
	return lo.Map(records, func(r consentrecords.Record, _ int) consentrecord.Source { return r.Source })
}
