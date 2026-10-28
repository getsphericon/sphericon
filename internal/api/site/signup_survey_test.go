package site_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/signupsurveyresponse"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const surveyEmail = "newcomer@survey.example"

// surveyOn is an Instance that asks the Sign-up survey (the switch and the `staff` license the
// test license carries) and has no email verification gate, so a registration is a whole sign-up.
func surveyOn(t *testing.T, opts ...testhelper.Option) *testhelper.TestEnv {
	t.Helper()
	on := testhelper.WithConfig(func(c *config.Config) {
		c.SignupSurvey = true
		c.RequireEmailVerification = false
	})
	return testhelper.Setup(t, append([]testhelper.Option{on}, opts...)...)
}

func signUpForSurvey(t *testing.T, env *testhelper.TestEnv) *siteapi.Client {
	t.Helper()
	res, err := env.SiteAnonymous(t).SiteAuthRegister(t.Context(),
		&siteapi.SiteRegisterInput{Name: "Newcomer", Email: surveyEmail, Password: siteapi.NewOptString(formPassword)})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteAuthRegisterAccepted{}, res)
	return env.SiteActor(t, surveyEmail)
}

func surveyDue(t *testing.T, c *siteapi.Client) bool {
	t.Helper()
	me, err := c.SiteUserGetMe(t.Context())
	require.NoError(t, err)
	return me.SignupSurveyDue
}

func validAnswers() *siteapi.SiteSignupSurveyInput {
	return &siteapi.SiteSignupSurveyInput{
		Role:        siteapi.SiteSignupRoleMarketer,
		CompanySize: siteapi.SiteSignupCompanySize210,
		MainGoal:    siteapi.SiteSignupGoalBroadcasts,
	}
}

func submitSurvey(t *testing.T, c *siteapi.Client, in *siteapi.SiteSignupSurveyInput) siteapi.SiteSignupSurveySubmitRes {
	t.Helper()
	res, err := c.SiteSignupSurveySubmit(t.Context(), in)
	require.NoError(t, err)
	return res
}

func responseOf(t *testing.T, env *testhelper.TestEnv, email string) *ent.SignupSurveyResponse {
	t.Helper()
	u := userByEmail(t, env, email)
	row, err := env.DB.SignupSurveyResponse.Query().Where(signupsurveyresponse.UserID(u.ID)).Only(t.Context())
	require.NoError(t, err)
	return row
}

func hasResponse(t *testing.T, env *testhelper.TestEnv, email string) bool {
	t.Helper()
	u := userByEmail(t, env, email)
	exists, err := env.DB.SignupSurveyResponse.Query().Where(signupsurveyresponse.UserID(u.ID)).Exist(t.Context())
	require.NoError(t, err)
	return exists
}

func TestAPasswordSignUpOwesTheSurveyUntilItIsSubmitted(t *testing.T) {
	env := surveyOn(t)
	newcomer := signUpForSurvey(t, env)
	dueAfterSignUp := surveyDue(t, newcomer)

	res := submitSurvey(t, newcomer, validAnswers())
	dueAfterSubmit := surveyDue(t, newcomer)

	assert.True(t, dueAfterSignUp)
	assert.IsType(t, &siteapi.SiteSignupSurveySubmitNoContent{}, res)
	assert.False(t, dueAfterSubmit)
}

func TestSubmittingStoresTheAnswersOfTheUser(t *testing.T) {
	env := surveyOn(t)
	newcomer := signUpForSurvey(t, env)
	in := validAnswers()
	in.HeardFrom = siteapi.NewOptSiteSignupSource(siteapi.SiteSignupSourceOther)
	in.HeardFromOther = siteapi.NewOptString("a podcast")

	submitSurvey(t, newcomer, in)

	row := responseOf(t, env, surveyEmail)
	assert.Equal(t, signupsurveyresponse.RoleMarketer, row.Role)
	assert.Equal(t, signupsurveyresponse.CompanySize210, row.CompanySize)
	assert.Equal(t, signupsurveyresponse.MainGoalBroadcasts, row.MainGoal)
	assert.Equal(t, signupsurveyresponse.HeardFromOther, *row.HeardFrom)
	assert.Equal(t, "a podcast", *row.HeardFromOtherText)
}

func TestTheWhereDidYouHearAnswerIsOptional(t *testing.T) {
	env := surveyOn(t)
	newcomer := signUpForSurvey(t, env)

	submitSurvey(t, newcomer, validAnswers())

	row := responseOf(t, env, surveyEmail)
	assert.Nil(t, row.HeardFrom)
}

func TestASecondSubmitIsRefusedAndKeepsTheFirstAnswers(t *testing.T) {
	env := surveyOn(t)
	newcomer := signUpForSurvey(t, env)
	submitSurvey(t, newcomer, validAnswers())
	again := validAnswers()
	again.Role = siteapi.SiteSignupRoleFounder

	res := submitSurvey(t, newcomer, again)

	assert.IsType(t, &siteapi.SiteSignupSurveySubmitConflict{}, res)
	row := responseOf(t, env, surveyEmail)
	assert.Equal(t, signupsurveyresponse.RoleMarketer, row.Role)
}

func TestAUserWhoAnsweredIsNeverDueAndCannotSubmitAgain(t *testing.T) {
	env := surveyOn(t)
	ann := env.SiteActor(t, fixtures.SurveyAnsweredAnnEmail)

	due := surveyDue(t, ann)
	res := submitSurvey(t, ann, validAnswers())

	assert.False(t, due)
	assert.IsType(t, &siteapi.SiteSignupSurveySubmitConflict{}, res)
}

func TestAnInstanceWithTheSwitchOffNeverAsks(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.RequireEmailVerification = false }))
	newcomer := signUpForSurvey(t, env)

	due := surveyDue(t, newcomer)
	res := submitSurvey(t, newcomer, validAnswers())

	assert.False(t, due)
	assert.IsType(t, &siteapi.SiteSignupSurveySubmitConflict{}, res)
}

func TestAnInstanceWithoutTheStaffLicenseNeverAsks(t *testing.T) {
	env := surveyOn(t, testhelper.WithoutLicense())
	newcomer := signUpForSurvey(t, env)

	due := surveyDue(t, newcomer)

	assert.False(t, due)
}

func TestAUserWhoVerifiesTheirAddressOwesTheSurveyToo(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.SignupSurvey = true }))
	registerAndVerify(t, env, "Grace Hopper", "grace@example.com", formPassword)

	due := surveyDue(t, env.SiteActor(t, "grace@example.com"))

	assert.True(t, due)
}

func TestAnInvitedUserIsNeverAsked(t *testing.T) {
	env := surveyOn(t)
	token := inviteToAcme(t, env, surveyEmail)
	res, err := env.SiteAnonymous(t).SitePublicInvitationsAccept(t.Context(),
		&siteapi.SiteAcceptInvitationInput{Name: siteapi.NewOptUserName("Newcomer"), Password: siteapi.NewOptString(formPassword)},
		siteapi.SitePublicInvitationsAcceptParams{Token: token})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SitePublicInvitationsAcceptOK{}, res)

	due := surveyDue(t, env.SiteActor(t, surveyEmail))

	assert.False(t, due)
}

func TestTheFirstOwnerMadeByTheOperatorIsNeverAsked(t *testing.T) {
	env := surveyOn(t)
	_, err := accounts.New(env.DB, env.Bus).CreateOwner(t.Context(), surveyEmail, "Ada Lovelace")
	require.NoError(t, err)

	due := surveyDue(t, env.SiteActor(t, surveyEmail))

	assert.False(t, due)
}

func TestAUserThatExistedBeforeTheSwitchIsNeverAsked(t *testing.T) {
	env := surveyOn(t)

	due := surveyDue(t, env.SiteActor(t, fixtures.MemberMaryEmail))

	assert.False(t, due)
}

func TestASubmitOutsideTheContractStoresNothingAndKeepsTheSurveyDue(t *testing.T) {
	cases := map[string]*siteapi.SiteSignupSurveyInput{
		"missing role":         {CompanySize: siteapi.SiteSignupCompanySize210, MainGoal: siteapi.SiteSignupGoalTracking},
		"unknown company size": {Role: siteapi.SiteSignupRoleFounder, CompanySize: "huge", MainGoal: siteapi.SiteSignupGoalTracking},
		"missing goal":         {Role: siteapi.SiteSignupRoleFounder, CompanySize: siteapi.SiteSignupCompanySize210},
		"unknown source": {
			Role: siteapi.SiteSignupRoleFounder, CompanySize: siteapi.SiteSignupCompanySize210, MainGoal: siteapi.SiteSignupGoalTracking,
			HeardFrom: siteapi.NewOptSiteSignupSource("tv"),
		},
		"over-long other": {
			Role: siteapi.SiteSignupRoleOther, RoleOther: siteapi.NewOptString(strings.Repeat("x", 101)),
			CompanySize: siteapi.SiteSignupCompanySize210, MainGoal: siteapi.SiteSignupGoalTracking,
		},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			env := surveyOn(t)
			newcomer := signUpForSurvey(t, env)

			_, err := newcomer.SiteSignupSurveySubmit(t.Context(), in)

			require.Error(t, err)
			assert.False(t, hasResponse(t, env, surveyEmail))
			assert.True(t, surveyDue(t, newcomer))
		})
	}
}

func TestAnOtherTextWithoutItsOtherChoiceIsRefusedAndStoresNothing(t *testing.T) {
	env := surveyOn(t)
	newcomer := signUpForSurvey(t, env)
	in := validAnswers()
	in.RoleOther = siteapi.NewOptString("something")

	res := submitSurvey(t, newcomer, in)

	assert.IsType(t, &siteapi.SiteSignupSurveySubmitUnprocessableEntity{}, res)
	assert.False(t, hasResponse(t, env, surveyEmail))
	assert.True(t, surveyDue(t, newcomer))
}

func TestDeletingTheUserRemovesTheirSurveyAnswers(t *testing.T) {
	env := surveyOn(t)
	ann := env.SiteActor(t, fixtures.SurveyAnsweredAnnEmail)

	res := deleteUser(t, ann, &siteapi.SiteDeleteUserInput{
		Email:           fixtures.SurveyAnsweredAnnEmail,
		CurrentPassword: siteapi.NewOptString("password"),
	})

	require.IsType(t, &siteapi.SiteUserDeleteUserNoContent{}, res)
	left, err := env.DB.SignupSurveyResponse.Get(t.Context(), fixtures.AnnSurveyResponseID)
	assert.Nil(t, left)
	assert.True(t, ent.IsNotFound(err))
}

func TestAnUnverifiedRegistrantWhoAcceptsAnInvitationIsNeverAsked(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.SignupSurvey = true }))
	register(t, env, "Newcomer", surveyEmail)
	token := inviteToAcme(t, env, surveyEmail)

	res, err := env.SiteAnonymous(t).SitePublicInvitationsAccept(t.Context(),
		&siteapi.SiteAcceptInvitationInput{Name: siteapi.NewOptUserName("Newcomer"), Password: siteapi.NewOptString(formPassword)},
		siteapi.SitePublicInvitationsAcceptParams{Token: token})

	require.NoError(t, err)
	require.IsType(t, &siteapi.SitePublicInvitationsAcceptOK{}, res)
	assert.False(t, surveyDue(t, env.SiteActor(t, surveyEmail)))
}

func TestAMarkedUserIsDueOnlyWhileTheInstanceAsks(t *testing.T) {
	switchOn := testhelper.WithConfig(func(c *config.Config) { c.SignupSurvey = true })
	cases := map[string]struct {
		opts []testhelper.Option
		due  bool
	}{
		"the switch is on":       {opts: []testhelper.Option{switchOn}, due: true},
		"the switch is off":      {due: false},
		"the license is missing": {opts: []testhelper.Option{switchOn, testhelper.WithoutLicense()}, due: false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t, c.opts...)

			due := surveyDue(t, env.SiteActor(t, fixtures.SurveyDueDanEmail))

			assert.Equal(t, c.due, due)
		})
	}
}

func TestASubmitIsRefusedWhenTheInstanceNoLongerAsks(t *testing.T) {
	env := testhelper.Setup(t)
	dan := env.SiteActor(t, fixtures.SurveyDueDanEmail)

	res := submitSurvey(t, dan, validAnswers())

	assert.IsType(t, &siteapi.SiteSignupSurveySubmitConflict{}, res)
	assert.False(t, hasResponse(t, env, fixtures.SurveyDueDanEmail))
}

func TestABlankOtherTextIsRefusedAndStoresNothing(t *testing.T) {
	env := surveyOn(t)
	newcomer := signUpForSurvey(t, env)
	in := validAnswers()
	in.Role = siteapi.SiteSignupRoleOther
	in.RoleOther = siteapi.NewOptString("   ")

	res := submitSurvey(t, newcomer, in)

	assert.IsType(t, &siteapi.SiteSignupSurveySubmitUnprocessableEntity{}, res)
	assert.False(t, hasResponse(t, env, surveyEmail))
	assert.True(t, surveyDue(t, newcomer))
}
