package site_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	entmembership "github.com/getsphericon/sphericon/ent/membership"
	entuser "github.com/getsphericon/sphericon/ent/user"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const formPassword = "form-password-1"

// signUpIn drives the Sign-up from one browser (a client with a cookie jar, see
// testhelper.SiteBrowser) and returns the token carried by the verification email it sent.
func signUpIn(t *testing.T, env *testhelper.TestEnv, browser *siteapi.Client, name, email, password string) string {
	t.Helper()
	_, err := browser.SiteAuthSignUp(t.Context(), &siteapi.SiteSignUpInput{
		Name: siteapi.UserName(name), Email: siteapi.EmailAddress(email), Password: siteapi.NewOptString(password),
	})
	require.NoError(t, err)
	_, body := lastSystemEmail(t, env)
	return tokenFromEmail(t, body)
}

// signUpLink signs up from a browser that is not kept: the cookie it is given is lost, so
// the link opened by verify (another browser) asks for a password.
func signUpLink(t *testing.T, env *testhelper.TestEnv, name, email string) string {
	t.Helper()
	return signUpIn(t, env, env.SiteAnonymous(t), name, email, formPassword)
}

// verifyIn follows the verification link in one browser; an empty password sends none.
func verifyIn(t *testing.T, browser *siteapi.Client, token, password string) *siteapi.SiteVerifyEmailResultHeaders {
	t.Helper()
	res, err := browser.SiteAuthVerifyEmail(t.Context(), verifyInput(token, password), siteapi.SiteAuthVerifyEmailParams{})
	require.NoError(t, err)
	return res
}

// tryVerifyIn follows the verification link in one browser, for a test that expects the refusal.
func tryVerifyIn(t *testing.T, browser *siteapi.Client, token, password string) error {
	t.Helper()
	_, err := browser.SiteAuthVerifyEmail(t.Context(), verifyInput(token, password), siteapi.SiteAuthVerifyEmailParams{})
	return err
}

func verifyInput(token, password string) *siteapi.SiteVerifyEmailInput {
	input := &siteapi.SiteVerifyEmailInput{Token: token}
	if password != "" {
		input.Password = siteapi.NewOptString(password)
	}
	return input
}

// verify follows the verification link in a browser that never signed up, sending a password.
func verify(t *testing.T, env *testhelper.TestEnv, token, password string) *siteapi.SiteVerifyEmailResultHeaders {
	t.Helper()
	return verifyIn(t, env.SiteAnonymous(t), token, password)
}

// tryVerify is verify for a test that expects the refusal.
func tryVerify(t *testing.T, env *testhelper.TestEnv, token, password string) error {
	t.Helper()
	return tryVerifyIn(t, env.SiteAnonymous(t), token, password)
}

func userByEmail(t *testing.T, env *testhelper.TestEnv, email string) *ent.User {
	t.Helper()
	u, err := env.DB.User.Query().Where(entuser.Email(email)).Only(t.Context())
	require.NoError(t, err)
	return u
}

// signUpAndVerify takes a new person through the whole sign-up.
func signUpAndVerify(t *testing.T, env *testhelper.TestEnv, name, email, password string) {
	t.Helper()
	verify(t, env, signUpLink(t, env, name, email), password)
}

func TestSigningUpCreatesAnUnverifiedUserWithNothingElse(t *testing.T) {
	env := testhelper.Setup(t)

	signUpLink(t, env, "Grace Hopper", "grace@example.com")

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1, "only the verification email: the welcome waits for a proven address")
	assert.Equal(t, "grace@example.com", msgs[0].To)
	assert.Contains(t, msgs[0].Subject, "Verify")

	u, err := env.DB.User.Query().Where(entuser.Email("grace@example.com")).Only(t.Context())
	require.NoError(t, err)
	assert.NotEmpty(t, u.PasswordHash, "the Sign-up password waits, hashed, on the Unverified User")
	assert.NotContains(t, u.PasswordHash, formPassword)
	assert.Nil(t, u.EmailVerifiedAt)
	memberships, err := env.DB.Membership.Query().Where(entmembership.UserID(u.ID)).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, memberships, "no Workspace yet")
}

func TestSigningUpValidatesNameAndEmail(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)

	_, err := c.SiteAuthSignUp(t.Context(), &siteapi.SiteSignUpInput{Name: "  ", Email: "blank@example.com"})
	problem := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, problem.Errors.Value, "name")
	assert.NotContains(t, problem.Errors.Value, "email")
	exists, err := env.DB.User.Query().Where(entuser.Email("blank@example.com")).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, exists, "a rejected Sign-up creates no user")
}

// An address no one can receive mail at and a password outside its length are refused per
// field, before any User or mail exists.
func TestSigningUpRefusesAnUnreachableAddressAndAShortPassword(t *testing.T) {
	env := testhelper.Setup(t)

	err := trySignUpWithPassword(t, env, "Grace Hopper", "grace@example", "1")
	problem := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, siteapi.ProblemDetailsErrors{
		"email":    {"enter an email address you can receive mail at, like name@example.com"},
		"password": {"the password must be 8 to 256 characters long"},
	}, problem.Errors.Value)
	assert.Equal(t, "check the email and password", problem.Detail.Value, "the detail names only the failed fields")
	exists, err := env.DB.User.Query().Where(entuser.Email("grace@example")).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, exists, "a rejected Sign-up creates no user")
	assert.Empty(t, env.SystemMail.Messages())
}

func TestASignUpFailingEveryFieldNamesThemAllInItsDetail(t *testing.T) {
	env := testhelper.Setup(t)

	err := trySignUpWithPassword(t, env, " ", "grace@example", "")
	problem := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, "check the name, email and password", problem.Detail.Value)
}

func TestLoginRefusesAnUnverifiedUserWithATypedAnswerAndNoSession(t *testing.T) {
	env := testhelper.Setup(t)
	signUpLink(t, env, "Grace Hopper", "grace@example.com")

	rec := postLogin(t, env, "grace@example.com", "any password at all")

	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	var body siteapi.ProblemDetails
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, siteapi.ProblemCodeEmailVerificationRequired, body.Code.Value)
	assert.Nil(t, sessionCookie(rec.Result()))
}

// The pre-hijacking shape: the User exists with a password the attacker knows. The
// gate does not look at the password, so the right one gains nothing, and no
// second-factor challenge is issued either.
func TestLoginRefusesAnUnverifiedUserEvenWithAPasswordAndASecondFactor(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, env.FeedDB.User.UpdateOneID(fixtures.SecondFactorSamID).ClearEmailVerifiedAt().Exec(t.Context()))

	rec := postLogin(t, env, fixtures.SecondFactorSamEmail, fixtures.SecondFactorSamPassword)

	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "challenge")
	assert.Nil(t, sessionCookie(rec.Result()))
}

func TestTheVerifyLinkSetsThePasswordCreatesTheWorkspaceAndAllowsLogin(t *testing.T) {
	env := testhelper.Setup(t)
	token := signUpLink(t, env, "Grace Hopper", "grace@example.com")

	verify(t, env, token, "grace-password-1")

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 2, "the welcome email follows the proof")
	assert.Equal(t, "grace@example.com", msgs[1].To)
	assert.Contains(t, msgs[1].Subject, "Welcome")

	rec := postLogin(t, env, "grace@example.com", "grace-password-1")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	cookie := sessionCookie(rec.Result())
	require.NotNil(t, cookie)

	spaces, err := env.SiteWithToken(t, cookie.Value).SiteWorkspacesList(t.Context())
	require.NoError(t, err)
	require.Len(t, spaces, 1)
	assert.Regexp(t, "^[0-9A-Za-z]{8}$", spaces[0].ID)
	assert.Equal(t, "Personal", spaces[0].Name)
}

// The link starts the session: the cookie it sets works on an authenticated /site call
// and the session epoch bump ended every session issued before it.
func TestTheVerifyLinkStartsASessionIntoTheFirstWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	token := signUpLink(t, env, "Grace Hopper", "grace@example.com")

	res := verify(t, env, token, "grace-password-1")
	cookie, err := http.ParseSetCookie(res.SetCookie)
	require.NoError(t, err)
	assert.Equal(t, "JWT", cookie.Name)
	assert.True(t, cookie.HttpOnly)

	spaces, err := env.SiteWithToken(t, cookie.Value).SiteWorkspacesList(t.Context())
	require.NoError(t, err)
	require.Len(t, spaces, 1)
	assert.Equal(t, users.FirstWorkspaceName, spaces[0].Name)
}

func TestTheVerifyLinkWorksOnce(t *testing.T) {
	env := testhelper.Setup(t)
	token := signUpLink(t, env, "Grace Hopper", "grace@example.com")
	verify(t, env, token, "grace-password-1")

	testhelper.SiteRefusal(t, tryVerify(t, env, token, "attacker-password-1"), http.StatusBadRequest, "a replayed link sets nothing")

	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, "grace@example.com", "attacker-password-1"))
	assert.Equal(t, http.StatusOK, loginStatus(t, env, "grace@example.com", "grace-password-1"))
}

func TestTheVerifyLinkRefusesBadInput(t *testing.T) {
	env := testhelper.Setup(t)
	signUpLink(t, env, "Grace Hopper", "grace@example.com")

	testhelper.SiteRefusal(t, tryVerify(t, env, "garbage", "grace-password-1"), http.StatusBadRequest)
	assert.Equal(t, http.StatusForbidden, loginStatus(t, env, "grace@example.com", "grace-password-1"),
		"a refused link leaves the User unverified")
}

func TestAnOldLinkNeverVerifiesAChangedAddress(t *testing.T) {
	env := testhelper.Setup(t)
	token := signUpLink(t, env, "Grace Hopper", "grace@example.com")
	u, err := env.DB.User.Query().Where(entuser.Email("grace@example.com")).Only(t.Context())
	require.NoError(t, err)
	require.NoError(t, env.FeedDB.User.UpdateOneID(u.ID).SetEmail("grace@elsewhere.example").Exec(t.Context()))

	testhelper.SiteRefusal(t, tryVerify(t, env, token, "grace-password-1"), http.StatusBadRequest)

	after, err := env.DB.User.Get(t.Context(), u.ID)
	require.NoError(t, err)
	assert.Nil(t, after.EmailVerifiedAt)
	assert.Equal(t, u.PasswordHash, after.PasswordHash)
}

func TestVerifyingIsRecordedInTheNewWorkspacesAuditLog(t *testing.T) {
	env := testhelper.Setup(t)
	signUpAndVerify(t, env, "Grace Hopper", "grace@example.com", "grace-password-1")

	got := entriesNamed(t, env, "grace@example.com", firstWorkspaceID(t, env, "grace@example.com"), events.ActionUserEmailVerify)

	require.Len(t, got, 1)
	assert.Equal(t, siteapi.SiteAuditActorKindUser, got[0].Actor.Kind)
	assert.Equal(t, "Grace Hopper", got[0].Actor.Name.Value)
	assert.Equal(t, "user", got[0].Target.Type)
}

// signUpWithPassword drives the Sign-up of an install with the gate off.
func signUpWithPassword(t *testing.T, env *testhelper.TestEnv, name, email, password string) *siteapi.SiteAuthSignUpAccepted {
	t.Helper()
	res, err := env.SiteAnonymous(t).SiteAuthSignUp(t.Context(), &siteapi.SiteSignUpInput{
		Name: siteapi.UserName(name), Email: siteapi.EmailAddress(email), Password: siteapi.NewOptString(password),
	})
	require.NoError(t, err)
	return res
}

// trySignUpWithPassword is signUpWithPassword for a test that expects the refusal.
func trySignUpWithPassword(t *testing.T, env *testhelper.TestEnv, name, email, password string) error {
	t.Helper()
	_, err := env.SiteAnonymous(t).SiteAuthSignUp(t.Context(), &siteapi.SiteSignUpInput{
		Name: siteapi.UserName(name), Email: siteapi.EmailAddress(email), Password: siteapi.NewOptString(password),
	})
	return err
}

func gateOff(t *testing.T) *testhelper.TestEnv {
	t.Helper()
	return testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.RequireEmailVerification = false }))
}

func TestWithTheGateOffSigningUpCreatesAVerifiedUserWithAWorkspaceAndSignInWorks(t *testing.T) {
	env := gateOff(t)

	signUpWithPassword(t, env, "Grace Hopper", "grace@example.com", "grace-password-1")

	u, err := env.DB.User.Query().Where(entuser.Email("grace@example.com")).Only(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, u.EmailVerifiedAt)
	assert.NotEmpty(t, u.PasswordHash)

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1, "no verification email: only the welcome")
	assert.Contains(t, msgs[0].Subject, "Welcome")

	rec := postLogin(t, env, "grace@example.com", "grace-password-1")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	cookie := sessionCookie(rec.Result())
	require.NotNil(t, cookie)
	spaces, err := env.SiteWithToken(t, cookie.Value).SiteWorkspacesList(t.Context())
	require.NoError(t, err)
	require.Len(t, spaces, 1)
	assert.Regexp(t, "^[0-9A-Za-z]{8}$", spaces[0].ID)
	assert.Equal(t, "Personal", spaces[0].Name)

	got := entriesNamed(t, env, "grace@example.com", firstWorkspaceID(t, env, "grace@example.com"), events.ActionUserEmailVerify)
	assert.Len(t, got, 1)
}

func TestWithTheGateOffSigningUpNeedsAPasswordAndRefusesATakenAddress(t *testing.T) {
	env := gateOff(t)

	missing := trySignUpWithPassword(t, env, "Grace Hopper", "grace@example.com", "")
	problem := testhelper.SiteRefusal(t, missing, http.StatusUnprocessableEntity)
	assert.Contains(t, problem.Errors.Value, "password")
	exists, err := env.DB.User.Query().Where(entuser.Email("grace@example.com")).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, exists, "a rejected Sign-up creates no user")

	dup := trySignUpWithPassword(t, env, "John again", fixtures.OwnerJohnEmail, "some-password-1")
	testhelper.SiteRefusal(t, dup, http.StatusConflict)
	assert.Empty(t, env.SystemMail.Messages())
}

func TestWithTheGateOffLoginHasNoVerificationCheck(t *testing.T) {
	env := gateOff(t)
	require.NoError(t, env.FeedDB.User.UpdateOneID(fixtures.OwnerJohnID).ClearEmailVerifiedAt().Exec(t.Context()))

	assert.Equal(t, http.StatusOK, loginStatus(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword))
}

// An Unverified User who lost the verification mail can recover through a password
// reset: the reset link reaches the same inbox, so it proves the address too (ADR 0030).
func TestPasswordResetProvesTheAddressOfAnUnverifiedUser(t *testing.T) {
	env := testhelper.Setup(t)
	signUpLink(t, env, "Grace Hopper", "grace@example.com")
	c := env.SiteAnonymous(t)

	err := c.SiteAuthForgotPassword(t.Context(), &siteapi.SiteForgotPasswordInput{Email: "grace@example.com"})
	require.NoError(t, err)
	_, body := lastSystemEmail(t, env)

	err = c.SiteAuthResetPassword(t.Context(),
		&siteapi.SiteResetPasswordInput{Token: tokenFromEmail(t, body), Password: "grace-password-2"})
	require.NoError(t, err)

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 3, "verification, reset, then the welcome that follows the proof")
	assert.Contains(t, msgs[2].Subject, "Welcome")

	rec := postLogin(t, env, "grace@example.com", "grace-password-2")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	spaces, err := env.SiteWithToken(t, sessionCookie(rec.Result()).Value).SiteWorkspacesList(t.Context())
	require.NoError(t, err)
	require.Len(t, spaces, 1)
	assert.Regexp(t, "^[0-9A-Za-z]{8}$", spaces[0].ID)
	assert.Equal(t, "Personal", spaces[0].Name)
}

// The reset of a Verified User is only a password change: no second Workspace, no welcome.
func TestPasswordResetOfAVerifiedUserCreatesNoWorkspaceAndSendsNoWelcome(t *testing.T) {
	env := testhelper.Setup(t)
	signUpAndVerify(t, env, "Grace Hopper", "grace@example.com", "grace-password-1")
	c := env.SiteAnonymous(t)
	err := c.SiteAuthForgotPassword(t.Context(), &siteapi.SiteForgotPasswordInput{Email: "grace@example.com"})
	require.NoError(t, err)
	_, body := lastSystemEmail(t, env)
	before := len(env.SystemMail.Messages())

	err = c.SiteAuthResetPassword(t.Context(),
		&siteapi.SiteResetPasswordInput{Token: tokenFromEmail(t, body), Password: "grace-password-2"})
	require.NoError(t, err)

	assert.Len(t, env.SystemMail.Messages(), before)
	rec := postLogin(t, env, "grace@example.com", "grace-password-2")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	spaces, err := env.SiteWithToken(t, sessionCookie(rec.Result()).Value).SiteWorkspacesList(t.Context())
	require.NoError(t, err)
	assert.Len(t, spaces, 1)
}

func resend(t *testing.T, env *testhelper.TestEnv, email string) error {
	t.Helper()
	return env.SiteAnonymous(t).SiteAuthResendVerification(t.Context(),
		&siteapi.SiteResendVerificationInput{Email: siteapi.EmailAddress(email)}, siteapi.SiteAuthResendVerificationParams{})
}

func TestResendSendsAFreshLinkThatCompletesTheSignUp(t *testing.T) {
	env := testhelper.Setup(t)
	signUpLink(t, env, "Grace Hopper", "grace@example.com")

	require.NoError(t, resend(t, env, "grace@example.com"))

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 2, "the Sign-up mail and the resent one")
	assert.Equal(t, "grace@example.com", msgs[1].To)
	assert.Contains(t, msgs[1].Subject, "Verify")
	fresh := tokenFromEmail(t, msgs[1].Text)

	verify(t, env, fresh, "grace-password-1")
	assert.Equal(t, http.StatusOK, loginStatus(t, env, "grace@example.com", "grace-password-1"))
}

func TestResendAnswersTheSameForUnknownAndVerifiedAddressesAndSendsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	signUpLink(t, env, "Grace Hopper", "grace@example.com")
	before := len(env.SystemMail.Messages())

	for _, email := range []string{"nobody@example.com", fixtures.OwnerJohnEmail, "grace@example.com"} {
		require.NoError(t, resend(t, env, email), email)
	}

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, before+1, "only the Unverified User gets a mail")
	assert.Equal(t, "grace@example.com", msgs[before].To)
}

func TestSettingThePasswordEndsEveryEarlierVerifyLinkAfterAResend(t *testing.T) {
	env := testhelper.Setup(t)
	first := signUpLink(t, env, "Grace Hopper", "grace@example.com")
	require.NoError(t, resend(t, env, "grace@example.com"))
	_, body := lastSystemEmail(t, env)
	fresh := tokenFromEmail(t, body)

	verify(t, env, fresh, "grace-password-1")
	testhelper.SiteRefusal(t, tryVerify(t, env, first, "attacker-password-1"), http.StatusBadRequest)
}

// Story 18: setting the password through the verify link bumps the session epoch,
// so no session issued before it survives (none can exist for an Unverified User, so
// the epoch itself is what is observable).
func TestVerifyingBumpsTheSessionEpoch(t *testing.T) {
	env := testhelper.Setup(t)
	token := signUpLink(t, env, "Grace Hopper", "grace@example.com")
	before, err := env.DB.User.Query().Where(entuser.Email("grace@example.com")).Only(t.Context())
	require.NoError(t, err)

	verify(t, env, token, "grace-password-1")

	after, err := env.DB.User.Get(t.Context(), before.ID)
	require.NoError(t, err)
	assert.Greater(t, after.SessionEpoch, before.SessionEpoch)
}

// Story 18: a password reset ends every session issued before it.
func TestPasswordResetEndsEveryEarlierSession(t *testing.T) {
	env := testhelper.Setup(t)
	signUpAndVerify(t, env, "Grace Hopper", "grace@example.com", "grace-password-1")
	rec := postLogin(t, env, "grace@example.com", "grace-password-1")
	require.Equal(t, http.StatusOK, rec.Code)
	earlier := sessionCookie(rec.Result()).Value
	require.Equal(t, http.StatusOK, workspacesStatus(t, env, earlier))

	c := env.SiteAnonymous(t)
	err := c.SiteAuthForgotPassword(t.Context(), &siteapi.SiteForgotPasswordInput{Email: "grace@example.com"})
	require.NoError(t, err)
	_, body := lastSystemEmail(t, env)
	err = c.SiteAuthResetPassword(t.Context(),
		&siteapi.SiteResetPasswordInput{Token: tokenFromEmail(t, body), Password: "grace-password-2"})
	require.NoError(t, err)

	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, earlier))
}

// signUpQuiet drives the public Sign-up without assuming what it answers.
func signUpQuiet(t *testing.T, env *testhelper.TestEnv, name, email string) error {
	t.Helper()
	_, err := env.SiteAnonymous(t).SiteAuthSignUp(t.Context(),
		&siteapi.SiteSignUpInput{Name: siteapi.UserName(name), Email: siteapi.EmailAddress(email), Password: siteapi.NewOptString(formPassword)})
	return err
}

// ADR 0031: a Sign-up answers the same for a new, an Unverified and a Verified address.
func TestSigningUpAnswersTheSameForEveryKindOfAddress(t *testing.T) {
	env := testhelper.Setup(t)
	signUpLink(t, env, "Grace Hopper", "grace@example.com")

	for _, email := range []string{"new@example.com", "grace@example.com", fixtures.OwnerJohnEmail} {
		assert.NoError(t, signUpQuiet(t, env, "Someone", email), email)
	}
}

func TestSigningUpAVerifiedAddressMailsTheOwnerATokenlessNoticeAndChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	before, err := env.DB.User.Get(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)

	require.NoError(t, signUpQuiet(t, env, "Mallory", fixtures.OwnerJohnEmail), "a taken address is accepted like a new one")

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, fixtures.OwnerJohnEmail, msgs[0].To)
	assert.Contains(t, msgs[0].Subject, "Someone tried")
	assert.NotContains(t, msgs[0].Text, "token=", "a notice must never carry a token")
	assert.Contains(t, msgs[0].Text, "/login")
	assert.Contains(t, msgs[0].Text, "/forgot-password")
	after, err := env.DB.User.Get(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.Equal(t, before.Name, after.Name)
	assert.Equal(t, before.PasswordHash, after.PasswordHash)
}

func TestSigningUpAnUnverifiedAddressAgainMailsAFreshLinkAndKeepsTheName(t *testing.T) {
	env := testhelper.Setup(t)
	signUpLink(t, env, "Grace Hopper", "grace@example.com")

	require.NoError(t, signUpQuiet(t, env, "Mallory", "grace@example.com"), "an unverified address is accepted again")

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 2)
	assert.Equal(t, "grace@example.com", msgs[1].To)
	assert.Contains(t, msgs[1].Subject, "Verify")
	u := userByEmail(t, env, "grace@example.com")
	assert.Equal(t, "Grace Hopper", u.Name)
}

func TestSigningUpStopsMailingAnAddressOnceItsBudgetIsSpent(t *testing.T) {
	const limit = 3
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{ForgotAddress: limit}))

	for range limit + 2 {
		require.NoError(t, signUpQuiet(t, env, "Mallory", fixtures.OwnerJohnEmail))
	}

	assert.Len(t, env.SystemMail.Messages(), limit, "the owner is not mail-bombed")
}

// A resend mails the Sign-up's link again, so it takes a slot of the same budget: a stranger who
// knows an Unverified address cannot make the Instance mail it without limit.
func TestResendingStopsMailingAnAddressOnceItsBudgetIsSpent(t *testing.T) {
	const limit = 3
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{ForgotAddress: limit}))
	signUpLink(t, env, "Grace Hopper", "grace@example.com")

	for range limit + 2 {
		require.NoError(t, resend(t, env, "grace@example.com"))
	}

	assert.Len(t, env.SystemMail.Messages(), limit, "the Sign-up and its resends share one budget")
}

// An unknown or a Verified address spends its budget as an Unverified one does, so the cost of a
// resend does not tell them apart: the Sign-up that follows finds the budget spent.
func TestResendingToAnyAddressSpendsItsBudget(t *testing.T) {
	const limit = 3
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{ForgotAddress: limit}))

	for _, email := range []string{"nobody@example.com", fixtures.OwnerJohnEmail} {
		for range limit {
			require.NoError(t, resend(t, env, email), email)
		}
		require.NoError(t, signUpQuiet(t, env, "Mallory", email), email)
	}

	assert.Empty(t, env.SystemMail.Messages(), "neither the verify link nor the notice to the owner")
}

func TestVerifyingAnEmailCreatesAWorkspaceNamedPersonalOwnedByTheUser(t *testing.T) {
	env := testhelper.Setup(t)

	signUpAndVerify(t, env, "Grace Hopper", "grace@example.com", "password123")
	signUpAndVerify(t, env, "Ada Lovelace", "ada@example.com", "password123")

	for _, email := range []string{"grace@example.com", "ada@example.com"} {
		u, err := env.DB.User.Query().Where(entuser.Email(email)).Only(t.Context())
		require.NoError(t, err)
		ms, err := env.DB.Membership.Query().Where(entmembership.UserID(u.ID)).WithWorkspace().All(t.Context())
		require.NoError(t, err)
		require.Len(t, ms, 1)
		assert.Equal(t, entmembership.RoleOwner, ms[0].Role)
		assert.Equal(t, "Personal", ms[0].Edges.Workspace.Name)
	}
}

// firstWorkspaceID is the id of the one Workspace the User has, as their own list shows it.
func firstWorkspaceID(t *testing.T, env *testhelper.TestEnv, email string) siteapi.WorkspaceId {
	t.Helper()
	spaces, err := env.SiteActor(t, email).SiteWorkspacesList(t.Context())
	require.NoError(t, err)
	require.Len(t, spaces, 1)
	return spaces[0].ID
}

func sessionOf(t *testing.T, res *siteapi.SiteVerifyEmailResultHeaders) *http.Cookie {
	t.Helper()
	cookie, err := http.ParseSetCookie(res.SetCookie)
	require.NoError(t, err)
	return cookie
}

func landsInFirstWorkspace(t *testing.T, env *testhelper.TestEnv, session *http.Cookie) {
	t.Helper()
	spaces, err := env.SiteWithToken(t, session.Value).SiteWorkspacesList(t.Context())
	require.NoError(t, err)
	require.Len(t, spaces, 1)
	assert.Equal(t, users.FirstWorkspaceName, spaces[0].Name)
}

func TestSigningUpNeedsAPassword(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.SiteAnonymous(t).SiteAuthSignUp(t.Context(),
		&siteapi.SiteSignUpInput{Name: "Grace Hopper", Email: "grace@example.com"})

	problem := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, problem.Errors.Value, "password")
	exists, err := env.DB.User.Query().Where(entuser.Email("grace@example.com")).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestASignUpSetsAScopedStrictHttpOnlyCookieForTheLifeOfTheLink(t *testing.T) {
	env := testhelper.Setup(t)

	accepted, err := env.SiteAnonymous(t).SiteAuthSignUp(t.Context(), &siteapi.SiteSignUpInput{
		Name: "Grace Hopper", Email: "grace@example.com", Password: siteapi.NewOptString(formPassword),
	})
	require.NoError(t, err)

	cookie, err := http.ParseSetCookie(accepted.SetCookie.Value)
	require.NoError(t, err)
	assert.NotEmpty(t, cookie.Value)
	assert.True(t, cookie.HttpOnly)
	assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
	assert.Equal(t, "/site/auth", cookie.Path)
	assert.Equal(t, 24*60*60, cookie.MaxAge)
}

func TestTheLinkOpenedInTheSignUpBrowserKeepsThePasswordAndSignsIn(t *testing.T) {
	env := testhelper.Setup(t)
	browser := env.SiteBrowser(t)
	token := signUpIn(t, env, browser, "Grace Hopper", "grace@example.com", formPassword)

	res := verifyIn(t, browser, token, "")

	session := sessionOf(t, res)
	landsInFirstWorkspace(t, env, session)
	assert.Equal(t, http.StatusOK, loginStatus(t, env, "grace@example.com", formPassword))
}

func TestTheLinkInAnotherBrowserWithoutAPasswordNeedsOneAndChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	token := signUpIn(t, env, env.SiteBrowser(t), "Grace Hopper", "grace@example.com", formPassword)
	before := userByEmail(t, env, "grace@example.com")
	mails := len(env.SystemMail.Messages())

	err := tryVerifyIn(t, env.SiteBrowser(t), token, "")

	testhelper.SiteRefusal(t, err, http.StatusPreconditionRequired)
	after := userByEmail(t, env, "grace@example.com")
	assert.Nil(t, after.EmailVerifiedAt)
	assert.Equal(t, before.PasswordHash, after.PasswordHash)
	assert.Equal(t, before.SessionEpoch, after.SessionEpoch)
	assert.Len(t, env.SystemMail.Messages(), mails, "no welcome yet")
	memberships, err := env.DB.Membership.Query().Where(entmembership.UserID(after.ID)).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, memberships)

	again := tryVerifyIn(t, env.SiteBrowser(t), token, "grace-password-2")
	assert.NoError(t, again, "the scanner-style request consumed nothing")
}

func TestTheLinkInAnotherBrowserWithAPasswordMakesThatPasswordLiveAndDiscardsTheFormOne(t *testing.T) {
	env := testhelper.Setup(t)
	token := signUpIn(t, env, env.SiteBrowser(t), "Grace Hopper", "grace@example.com", formPassword)

	res := verifyIn(t, env.SiteBrowser(t), token, "grace-password-2")

	session := sessionOf(t, res)
	landsInFirstWorkspace(t, env, session)
	assert.Equal(t, http.StatusOK, loginStatus(t, env, "grace@example.com", "grace-password-2"))
	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, "grace@example.com", formPassword))
}

// Pre-hijacking: the attacker signs up the victim's address with a password they know;
// the victim follows the mail in their own browser and sets their own.
func TestAnAttackersPasswordNeverGoesLiveWhenTheVictimFollowsTheLink(t *testing.T) {
	env := testhelper.Setup(t)
	attacker := env.SiteBrowser(t)
	token := signUpIn(t, env, attacker, "Victim", "victim@example.com", "attacker-password-1")
	victim := env.SiteBrowser(t)

	testhelper.SiteRefusal(t, tryVerifyIn(t, victim, token, ""), http.StatusPreconditionRequired)
	verifyIn(t, victim, token, "victim-password-1")

	assert.Equal(t, http.StatusOK, loginStatus(t, env, "victim@example.com", "victim-password-1"))
	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, "victim@example.com", "attacker-password-1"))
}

func TestSigningUpTheSameUnverifiedAddressTwiceLeavesOnlyTheLatestMailAndPassword(t *testing.T) {
	env := testhelper.Setup(t)
	browser := env.SiteBrowser(t)
	first := signUpIn(t, env, browser, "Grace Hopper", "grace@example.com", "first-password-1")
	latest := signUpIn(t, env, browser, "Grace Hopper", "grace@example.com", "latest-password-1")

	testhelper.SiteRefusal(t, tryVerifyIn(t, browser, first, ""), http.StatusBadRequest, "the earlier mail is dead")
	testhelper.SiteRefusal(t, tryVerifyIn(t, browser, first, "attacker-password-1"), http.StatusBadRequest)
	verifyIn(t, browser, latest, "")

	assert.Equal(t, http.StatusOK, loginStatus(t, env, "grace@example.com", "latest-password-1"))
	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, "grace@example.com", "first-password-1"))
}

// ADR 0031: a Verified address answers the same as a new one, cookie included, and its
// password is not stored.
func TestSigningUpAVerifiedAddressSetsTheSameCookieAndStoresNothing(t *testing.T) {
	env := testhelper.Setup(t)
	before, err := env.DB.User.Get(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	cookieOf := func(email string) *http.Cookie {
		accepted, err := env.SiteAnonymous(t).SiteAuthSignUp(t.Context(), &siteapi.SiteSignUpInput{
			Name: "Mallory", Email: siteapi.EmailAddress(email), Password: siteapi.NewOptString("mallory-password-1"),
		})
		require.NoError(t, err)
		cookie, err := http.ParseSetCookie(accepted.SetCookie.Value)
		require.NoError(t, err)
		return cookie
	}

	fresh := cookieOf("new@example.com")
	taken := cookieOf(fixtures.OwnerJohnEmail)

	assert.Equal(t, fresh.Name, taken.Name)
	assert.Equal(t, fresh.Path, taken.Path)
	assert.Equal(t, fresh.MaxAge, taken.MaxAge)
	assert.Equal(t, fresh.SameSite, taken.SameSite)
	assert.Equal(t, fresh.HttpOnly, taken.HttpOnly)
	assert.Equal(t, fresh.Secure, taken.Secure)
	assert.NotEmpty(t, taken.Value)
	after, err := env.DB.User.Get(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	assert.Equal(t, before.PasswordHash, after.PasswordHash)
	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, fixtures.OwnerJohnEmail, "mallory-password-1"))
}

// resendIn asks for the link again from one browser and returns the token of the new mail.
func resendIn(t *testing.T, env *testhelper.TestEnv, browser *siteapi.Client, email string) string {
	t.Helper()
	err := browser.SiteAuthResendVerification(t.Context(), &siteapi.SiteResendVerificationInput{Email: siteapi.EmailAddress(email)},
		siteapi.SiteAuthResendVerificationParams{})
	require.NoError(t, err)
	_, body := lastSystemEmail(t, env)
	return tokenFromEmail(t, body)
}

func TestResendFromTheSignUpBrowserGivesALinkThatKeepsThePasswordAndSignsIn(t *testing.T) {
	env := testhelper.Setup(t)
	browser := env.SiteBrowser(t)
	signUpIn(t, env, browser, "Grace Hopper", "grace@example.com", formPassword)
	resent := resendIn(t, env, browser, "grace@example.com")

	session := sessionOf(t, verifyIn(t, browser, resent, ""))

	landsInFirstWorkspace(t, env, session)
	assert.Equal(t, http.StatusOK, loginStatus(t, env, "grace@example.com", formPassword))
}

func TestResendFromABrowserWithoutTheCookieGivesALinkThatAsksForAPassword(t *testing.T) {
	env := testhelper.Setup(t)
	signUpIn(t, env, env.SiteBrowser(t), "Grace Hopper", "grace@example.com", formPassword)
	resent := resendIn(t, env, env.SiteBrowser(t), "grace@example.com")
	browser := env.SiteBrowser(t)

	testhelper.SiteRefusal(t, tryVerifyIn(t, browser, resent, ""), http.StatusPreconditionRequired)
	require.NoError(t, tryVerifyIn(t, browser, resent, "grace-password-2"))
	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, "grace@example.com", formPassword))
}

// Story 24: verifying ends every session issued before it.
func TestVerifyingEndsASessionIssuedBeforeIt(t *testing.T) {
	env := testhelper.Setup(t)
	token := signUpLink(t, env, "Grace Hopper", "grace@example.com")
	earlier := env.SiteToken(t, "grace@example.com", nil)

	verify(t, env, token, "grace-password-1")

	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, earlier))
}

func TestABrowsersCookieDoesNotVouchForAnotherSignUpsLink(t *testing.T) {
	env := testhelper.Setup(t)
	browser := env.SiteBrowser(t)
	token := signUpIn(t, env, browser, "Grace Hopper", "grace@example.com", formPassword)

	// The link of a Sign-up made in another browser still needs a password.
	other := signUpLink(t, env, "Alan Turing", "alan@example.com")
	testhelper.SiteRefusal(t, tryVerifyIn(t, browser, other, ""), http.StatusPreconditionRequired)
	assert.NoError(t, tryVerifyIn(t, browser, token, ""))
}
