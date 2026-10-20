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
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const formPassword = "form-password-1"

// registerIn drives the registration from one browser (a client with a cookie jar, see
// testhelper.SiteBrowser) and returns the token carried by the verification email it sent.
func registerIn(t *testing.T, env *testhelper.TestEnv, browser *siteapi.Client, name, email, password string) string {
	t.Helper()
	res, err := browser.SiteAuthRegister(t.Context(), &siteapi.SiteRegisterInput{
		Name: name, Email: siteapi.EmailAddress(email), Password: siteapi.NewOptString(password),
	})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteAuthRegisterAccepted{}, res)
	_, body := lastSystemEmail(t, env)
	return tokenFromEmail(t, body)
}

// register registers from a browser that is not kept: the cookie it is given is lost, so
// the link opened by verify (another browser) asks for a password.
func register(t *testing.T, env *testhelper.TestEnv, name, email string) string {
	t.Helper()
	return registerIn(t, env, env.SiteAnonymous(t), name, email, formPassword)
}

// verifyIn follows the verification link in one browser; an empty password sends none.
func verifyIn(t *testing.T, browser *siteapi.Client, token, password string) siteapi.SiteAuthVerifyEmailRes {
	t.Helper()
	input := &siteapi.SiteVerifyEmailInput{Token: token}
	if password != "" {
		input.Password = siteapi.NewOptString(password)
	}
	res, err := browser.SiteAuthVerifyEmail(t.Context(), input, siteapi.SiteAuthVerifyEmailParams{})
	require.NoError(t, err)
	return res
}

// verify follows the verification link in a browser that never registered, sending a password.
func verify(t *testing.T, env *testhelper.TestEnv, token, password string) siteapi.SiteAuthVerifyEmailRes {
	t.Helper()
	return verifyIn(t, env.SiteAnonymous(t), token, password)
}

func userByEmail(t *testing.T, env *testhelper.TestEnv, email string) *ent.User {
	t.Helper()
	u, err := env.DB.User.Query().Where(entuser.Email(email)).Only(t.Context())
	require.NoError(t, err)
	return u
}

// registerAndVerify takes a new person through the whole sign-up.
func registerAndVerify(t *testing.T, env *testhelper.TestEnv, name, email, password string) {
	t.Helper()
	require.IsType(t, &siteapi.SiteAuthVerifyEmailOK{}, verify(t, env, register(t, env, name, email), password))
}

func TestRegisteringCreatesAnUnverifiedUserWithNothingElse(t *testing.T) {
	env := testhelper.Setup(t)

	register(t, env, "Grace Hopper", "grace@example.com")

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1, "only the verification email: the welcome waits for a proven address")
	assert.Equal(t, "grace@example.com", msgs[0].To)
	assert.Contains(t, msgs[0].Subject, "Verify")

	u, err := env.DB.User.Query().Where(entuser.Email("grace@example.com")).Only(t.Context())
	require.NoError(t, err)
	assert.NotEmpty(t, u.PasswordHash, "the registration password waits, hashed, on the Unverified User")
	assert.NotContains(t, u.PasswordHash, formPassword)
	assert.Nil(t, u.EmailVerifiedAt)
	memberships, err := env.DB.Membership.Query().Where(entmembership.UserID(u.ID)).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, memberships, "no Workspace yet")
}

func TestRegisteringValidatesNameAndEmail(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)

	blank, err := c.SiteAuthRegister(t.Context(), &siteapi.SiteRegisterInput{Name: "  ", Email: "blank@example.com"})
	require.NoError(t, err)
	problem, ok := blank.(*siteapi.SiteAuthRegisterUnprocessableEntity)
	require.Truef(t, ok, "got %T", blank)
	assert.Contains(t, problem.Errors.Value, "name")
	assert.NotContains(t, problem.Errors.Value, "email")
	exists, err := env.DB.User.Query().Where(entuser.Email("blank@example.com")).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, exists, "a rejected registration creates no user")

}

func TestLoginRefusesAnUnverifiedUserWithATypedAnswerAndNoSession(t *testing.T) {
	env := testhelper.Setup(t)
	register(t, env, "Grace Hopper", "grace@example.com")

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
	require.NoError(t, env.DB.User.UpdateOneID(fixtures.SecondFactorSamID).ClearEmailVerifiedAt().Exec(t.Context()))

	rec := postLogin(t, env, fixtures.SecondFactorSamEmail, fixtures.SecondFactorSamPassword)

	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "challenge")
	assert.Nil(t, sessionCookie(rec.Result()))
}

func TestTheVerifyLinkSetsThePasswordCreatesTheWorkspaceAndAllowsLogin(t *testing.T) {
	env := testhelper.Setup(t)
	token := register(t, env, "Grace Hopper", "grace@example.com")

	res := verify(t, env, token, "grace-password-1")
	require.IsType(t, &siteapi.SiteAuthVerifyEmailOK{}, res)

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
	assert.Equal(t, "grace-hopper", spaces[0].Slug)
	assert.Equal(t, "Grace Hopper", spaces[0].Name)
}

// The link starts the session: the cookie it sets works on an authenticated /site call
// and the session epoch bump ended every session issued before it.
func TestTheVerifyLinkStartsASessionIntoTheFirstWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	token := register(t, env, "Grace Hopper", "grace@example.com")

	res := verify(t, env, token, "grace-password-1")
	ok, isOK := res.(*siteapi.SiteAuthVerifyEmailOK)
	require.True(t, isOK, "got %T", res)
	cookie, err := http.ParseSetCookie(ok.SetCookie)
	require.NoError(t, err)
	assert.Equal(t, "JWT", cookie.Name)
	assert.True(t, cookie.HttpOnly)

	spaces, err := env.SiteWithToken(t, cookie.Value).SiteWorkspacesList(t.Context())
	require.NoError(t, err)
	require.Len(t, spaces, 1)
	assert.Equal(t, "grace-hopper", spaces[0].Slug)
}

func TestTheVerifyLinkWorksOnce(t *testing.T) {
	env := testhelper.Setup(t)
	token := register(t, env, "Grace Hopper", "grace@example.com")
	require.IsType(t, &siteapi.SiteAuthVerifyEmailOK{}, verify(t, env, token, "grace-password-1"))

	assert.IsType(t, &siteapi.SiteAuthVerifyEmailBadRequest{}, verify(t, env, token, "attacker-password-1"), "a replayed link sets nothing")

	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, "grace@example.com", "attacker-password-1"))
	assert.Equal(t, http.StatusOK, loginStatus(t, env, "grace@example.com", "grace-password-1"))
}

func TestTheVerifyLinkRefusesBadInput(t *testing.T) {
	env := testhelper.Setup(t)
	register(t, env, "Grace Hopper", "grace@example.com")

	assert.IsType(t, &siteapi.SiteAuthVerifyEmailBadRequest{}, verify(t, env, "garbage", "grace-password-1"))
	assert.Equal(t, http.StatusForbidden, loginStatus(t, env, "grace@example.com", "grace-password-1"),
		"a refused link leaves the User unverified")
}

func TestAnOldLinkNeverVerifiesAChangedAddress(t *testing.T) {
	env := testhelper.Setup(t)
	token := register(t, env, "Grace Hopper", "grace@example.com")
	u, err := env.DB.User.Query().Where(entuser.Email("grace@example.com")).Only(t.Context())
	require.NoError(t, err)
	require.NoError(t, env.DB.User.UpdateOneID(u.ID).SetEmail("grace@elsewhere.example").Exec(t.Context()))

	assert.IsType(t, &siteapi.SiteAuthVerifyEmailBadRequest{}, verify(t, env, token, "grace-password-1"))

	after, err := env.DB.User.Get(t.Context(), u.ID)
	require.NoError(t, err)
	assert.Nil(t, after.EmailVerifiedAt)
	assert.Equal(t, u.PasswordHash, after.PasswordHash)
}

func TestVerifyingIsRecordedInTheNewWorkspacesAuditLog(t *testing.T) {
	env := testhelper.Setup(t)
	registerAndVerify(t, env, "Grace Hopper", "grace@example.com", "grace-password-1")

	got := entriesNamed(t, env, "grace@example.com", "grace-hopper", events.ActionUserEmailVerify)

	require.Len(t, got, 1)
	assert.Equal(t, siteapi.SiteAuditActorKindUser, got[0].Actor.Kind)
	assert.Equal(t, "Grace Hopper", got[0].Actor.Name.Value)
	assert.Equal(t, "user", got[0].Target.Type)
}

// registerWithPassword drives the registration of an install with the gate off.
func registerWithPassword(t *testing.T, env *testhelper.TestEnv, name, email, password string) siteapi.SiteAuthRegisterRes {
	t.Helper()
	res, err := env.SiteAnonymous(t).SiteAuthRegister(t.Context(), &siteapi.SiteRegisterInput{
		Name: name, Email: siteapi.EmailAddress(email), Password: siteapi.NewOptString(password),
	})
	require.NoError(t, err)
	return res
}

func gateOff(t *testing.T) *testhelper.TestEnv {
	t.Helper()
	return testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.RequireEmailVerification = false }))
}

func TestWithTheGateOffRegisteringCreatesAVerifiedUserWithAWorkspaceAndSignInWorks(t *testing.T) {
	env := gateOff(t)

	require.IsType(t, &siteapi.SiteAuthRegisterAccepted{}, registerWithPassword(t, env, "Grace Hopper", "grace@example.com", "grace-password-1"))

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
	assert.Equal(t, "grace-hopper", spaces[0].Slug)

	got := entriesNamed(t, env, "grace@example.com", "grace-hopper", events.ActionUserEmailVerify)
	assert.Len(t, got, 1)
}

func TestWithTheGateOffRegisteringNeedsAPasswordAndRefusesATakenAddress(t *testing.T) {
	env := gateOff(t)

	missing := registerWithPassword(t, env, "Grace Hopper", "grace@example.com", "")
	problem, ok := missing.(*siteapi.SiteAuthRegisterUnprocessableEntity)
	require.Truef(t, ok, "got %T", missing)
	assert.Contains(t, problem.Errors.Value, "password")
	exists, err := env.DB.User.Query().Where(entuser.Email("grace@example.com")).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, exists, "a rejected registration creates no user")

	dup := registerWithPassword(t, env, "John again", fixtures.OwnerJohnEmail, "some-password-1")
	assert.IsType(t, &siteapi.SiteAuthRegisterConflict{}, dup)
	assert.Empty(t, env.SystemMail.Messages())
}

func TestWithTheGateOffLoginHasNoVerificationCheck(t *testing.T) {
	env := gateOff(t)
	require.NoError(t, env.DB.User.UpdateOneID(fixtures.OwnerJohnID).ClearEmailVerifiedAt().Exec(t.Context()))

	assert.Equal(t, http.StatusOK, loginStatus(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword))
}

// An Unverified User who lost the verification mail can recover through a password
// reset: the reset link reaches the same inbox, so it proves the address too (ADR 0030).
func TestPasswordResetProvesTheAddressOfAnUnverifiedUser(t *testing.T) {
	env := testhelper.Setup(t)
	register(t, env, "Grace Hopper", "grace@example.com")
	c := env.SiteAnonymous(t)

	forgot, err := c.SiteAuthForgotPassword(t.Context(), &siteapi.SiteForgotPasswordInput{Email: "grace@example.com"})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteAuthForgotPasswordAccepted{}, forgot)
	_, body := lastSystemEmail(t, env)

	res, err := c.SiteAuthResetPassword(t.Context(),
		&siteapi.SiteResetPasswordInput{Token: tokenFromEmail(t, body), Password: "grace-password-2"})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteAuthResetPasswordOK{}, res)

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 3, "verification, reset, then the welcome that follows the proof")
	assert.Contains(t, msgs[2].Subject, "Welcome")

	rec := postLogin(t, env, "grace@example.com", "grace-password-2")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	spaces, err := env.SiteWithToken(t, sessionCookie(rec.Result()).Value).SiteWorkspacesList(t.Context())
	require.NoError(t, err)
	require.Len(t, spaces, 1)
	assert.Equal(t, "grace-hopper", spaces[0].Slug)
}

// The reset of a Verified User is only a password change: no second Workspace, no welcome.
func TestPasswordResetOfAVerifiedUserCreatesNoWorkspaceAndSendsNoWelcome(t *testing.T) {
	env := testhelper.Setup(t)
	registerAndVerify(t, env, "Grace Hopper", "grace@example.com", "grace-password-1")
	c := env.SiteAnonymous(t)
	_, err := c.SiteAuthForgotPassword(t.Context(), &siteapi.SiteForgotPasswordInput{Email: "grace@example.com"})
	require.NoError(t, err)
	_, body := lastSystemEmail(t, env)
	before := len(env.SystemMail.Messages())

	res, err := c.SiteAuthResetPassword(t.Context(),
		&siteapi.SiteResetPasswordInput{Token: tokenFromEmail(t, body), Password: "grace-password-2"})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteAuthResetPasswordOK{}, res)

	assert.Len(t, env.SystemMail.Messages(), before)
	rec := postLogin(t, env, "grace@example.com", "grace-password-2")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	spaces, err := env.SiteWithToken(t, sessionCookie(rec.Result()).Value).SiteWorkspacesList(t.Context())
	require.NoError(t, err)
	assert.Len(t, spaces, 1)
}

func resend(t *testing.T, env *testhelper.TestEnv, email string) siteapi.SiteAuthResendVerificationRes {
	t.Helper()
	res, err := env.SiteAnonymous(t).SiteAuthResendVerification(t.Context(),
		&siteapi.SiteResendVerificationInput{Email: siteapi.EmailAddress(email)})
	require.NoError(t, err)
	return res
}

func TestResendSendsAFreshLinkThatCompletesTheRegistration(t *testing.T) {
	env := testhelper.Setup(t)
	register(t, env, "Grace Hopper", "grace@example.com")

	require.IsType(t, &siteapi.SiteAuthResendVerificationAccepted{}, resend(t, env, "grace@example.com"))

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 2, "the registration mail and the resent one")
	assert.Equal(t, "grace@example.com", msgs[1].To)
	assert.Contains(t, msgs[1].Subject, "Verify")
	fresh := tokenFromEmail(t, msgs[1].Text)

	require.IsType(t, &siteapi.SiteAuthVerifyEmailOK{}, verify(t, env, fresh, "grace-password-1"))
	assert.Equal(t, http.StatusOK, loginStatus(t, env, "grace@example.com", "grace-password-1"))
}

func TestResendAnswersTheSameForUnknownAndVerifiedAddressesAndSendsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	register(t, env, "Grace Hopper", "grace@example.com")
	before := len(env.SystemMail.Messages())

	for _, email := range []string{"nobody@example.com", fixtures.OwnerJohnEmail, "grace@example.com"} {
		assert.IsType(t, &siteapi.SiteAuthResendVerificationAccepted{}, resend(t, env, email), email)
	}

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, before+1, "only the Unverified User gets a mail")
	assert.Equal(t, "grace@example.com", msgs[before].To)
}

func TestSettingThePasswordEndsEveryEarlierVerifyLinkAfterAResend(t *testing.T) {
	env := testhelper.Setup(t)
	first := register(t, env, "Grace Hopper", "grace@example.com")
	resend(t, env, "grace@example.com")
	_, body := lastSystemEmail(t, env)
	fresh := tokenFromEmail(t, body)

	require.IsType(t, &siteapi.SiteAuthVerifyEmailOK{}, verify(t, env, fresh, "grace-password-1"))
	assert.IsType(t, &siteapi.SiteAuthVerifyEmailBadRequest{}, verify(t, env, first, "attacker-password-1"))
}

// Story 18: setting the password through the verify link bumps the session epoch,
// so no session issued before it survives (none can exist for an Unverified User, so
// the epoch itself is what is observable).
func TestVerifyingBumpsTheSessionEpoch(t *testing.T) {
	env := testhelper.Setup(t)
	token := register(t, env, "Grace Hopper", "grace@example.com")
	before, err := env.DB.User.Query().Where(entuser.Email("grace@example.com")).Only(t.Context())
	require.NoError(t, err)

	require.IsType(t, &siteapi.SiteAuthVerifyEmailOK{}, verify(t, env, token, "grace-password-1"))

	after, err := env.DB.User.Get(t.Context(), before.ID)
	require.NoError(t, err)
	assert.Greater(t, after.SessionEpoch, before.SessionEpoch)
}

// Story 18: a password reset ends every session issued before it.
func TestPasswordResetEndsEveryEarlierSession(t *testing.T) {
	env := testhelper.Setup(t)
	registerAndVerify(t, env, "Grace Hopper", "grace@example.com", "grace-password-1")
	rec := postLogin(t, env, "grace@example.com", "grace-password-1")
	require.Equal(t, http.StatusOK, rec.Code)
	earlier := sessionCookie(rec.Result()).Value
	require.Equal(t, http.StatusOK, workspacesStatus(t, env, earlier))

	c := env.SiteAnonymous(t)
	_, err := c.SiteAuthForgotPassword(t.Context(), &siteapi.SiteForgotPasswordInput{Email: "grace@example.com"})
	require.NoError(t, err)
	_, body := lastSystemEmail(t, env)
	res, err := c.SiteAuthResetPassword(t.Context(),
		&siteapi.SiteResetPasswordInput{Token: tokenFromEmail(t, body), Password: "grace-password-2"})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteAuthResetPasswordOK{}, res)

	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, earlier))
}

// registerQuiet drives the public registration without assuming what it answers.
func registerQuiet(t *testing.T, env *testhelper.TestEnv, name, email string) siteapi.SiteAuthRegisterRes {
	t.Helper()
	res, err := env.SiteAnonymous(t).SiteAuthRegister(t.Context(),
		&siteapi.SiteRegisterInput{Name: name, Email: siteapi.EmailAddress(email), Password: siteapi.NewOptString(formPassword)})
	require.NoError(t, err)
	return res
}

// ADR 0031: a registration answers the same for a new, an Unverified and a Verified address.
func TestRegisteringAnswersTheSameForEveryKindOfAddress(t *testing.T) {
	env := testhelper.Setup(t)
	register(t, env, "Grace Hopper", "grace@example.com")

	for _, email := range []string{"new@example.com", "grace@example.com", fixtures.OwnerJohnEmail} {
		assert.IsType(t, &siteapi.SiteAuthRegisterAccepted{}, registerQuiet(t, env, "Someone", email), email)
	}
}

func TestRegisteringAVerifiedAddressMailsTheOwnerATokenlessNoticeAndChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	before, err := env.DB.User.Get(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)

	registerQuiet(t, env, "Mallory", fixtures.OwnerJohnEmail)

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

func TestRegisteringAnUnverifiedAddressAgainMailsAFreshLinkAndKeepsTheName(t *testing.T) {
	env := testhelper.Setup(t)
	register(t, env, "Grace Hopper", "grace@example.com")

	registerQuiet(t, env, "Mallory", "grace@example.com")

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 2)
	assert.Equal(t, "grace@example.com", msgs[1].To)
	assert.Contains(t, msgs[1].Subject, "Verify")
	u := userByEmail(t, env, "grace@example.com")
	assert.Equal(t, "Grace Hopper", u.Name)
}

func TestRegisteringStopsMailingAnAddressOnceItsBudgetIsSpent(t *testing.T) {
	const limit = 3
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{ForgotAddress: limit}))

	for range limit + 2 {
		assert.IsType(t, &siteapi.SiteAuthRegisterAccepted{}, registerQuiet(t, env, "Mallory", fixtures.OwnerJohnEmail))
	}

	assert.Len(t, env.SystemMail.Messages(), limit, "the owner is not mail-bombed")
}

func sessionOf(t *testing.T, res siteapi.SiteAuthVerifyEmailRes) *http.Cookie {
	t.Helper()
	ok, isOK := res.(*siteapi.SiteAuthVerifyEmailOK)
	require.Truef(t, isOK, "got %T", res)
	cookie, err := http.ParseSetCookie(ok.SetCookie)
	require.NoError(t, err)
	return cookie
}

func landsInFirstWorkspace(t *testing.T, env *testhelper.TestEnv, session *http.Cookie, slug string) {
	t.Helper()
	spaces, err := env.SiteWithToken(t, session.Value).SiteWorkspacesList(t.Context())
	require.NoError(t, err)
	require.Len(t, spaces, 1)
	assert.Equal(t, slug, spaces[0].Slug)
}

func TestRegisteringNeedsAPassword(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := env.SiteAnonymous(t).SiteAuthRegister(t.Context(),
		&siteapi.SiteRegisterInput{Name: "Grace Hopper", Email: "grace@example.com"})
	require.NoError(t, err)

	problem, ok := res.(*siteapi.SiteAuthRegisterUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, problem.Errors.Value, "password")
	exists, err := env.DB.User.Query().Where(entuser.Email("grace@example.com")).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestRegistrationSetsAScopedStrictHttpOnlyCookieForTheLifeOfTheLink(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := env.SiteAnonymous(t).SiteAuthRegister(t.Context(), &siteapi.SiteRegisterInput{
		Name: "Grace Hopper", Email: "grace@example.com", Password: siteapi.NewOptString(formPassword),
	})
	require.NoError(t, err)

	accepted, ok := res.(*siteapi.SiteAuthRegisterAccepted)
	require.Truef(t, ok, "got %T", res)
	cookie, err := http.ParseSetCookie(accepted.SetCookie.Value)
	require.NoError(t, err)
	assert.NotEmpty(t, cookie.Value)
	assert.True(t, cookie.HttpOnly)
	assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
	assert.Equal(t, "/site/auth/verify-email", cookie.Path)
	assert.Equal(t, 24*60*60, cookie.MaxAge)
}

func TestTheLinkOpenedInTheRegistrationBrowserKeepsThePasswordAndSignsIn(t *testing.T) {
	env := testhelper.Setup(t)
	browser := env.SiteBrowser(t)
	token := registerIn(t, env, browser, "Grace Hopper", "grace@example.com", formPassword)

	res := verifyIn(t, browser, token, "")

	session := sessionOf(t, res)
	landsInFirstWorkspace(t, env, session, "grace-hopper")
	assert.Equal(t, http.StatusOK, loginStatus(t, env, "grace@example.com", formPassword))
}

func TestTheLinkInAnotherBrowserWithoutAPasswordNeedsOneAndChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	token := registerIn(t, env, env.SiteBrowser(t), "Grace Hopper", "grace@example.com", formPassword)
	before := userByEmail(t, env, "grace@example.com")
	mails := len(env.SystemMail.Messages())

	res := verifyIn(t, env.SiteBrowser(t), token, "")

	assert.IsType(t, &siteapi.SiteAuthVerifyEmailPreconditionRequired{}, res)
	after := userByEmail(t, env, "grace@example.com")
	assert.Nil(t, after.EmailVerifiedAt)
	assert.Equal(t, before.PasswordHash, after.PasswordHash)
	assert.Equal(t, before.SessionEpoch, after.SessionEpoch)
	assert.Len(t, env.SystemMail.Messages(), mails, "no welcome yet")
	memberships, err := env.DB.Membership.Query().Where(entmembership.UserID(after.ID)).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, memberships)

	again := verifyIn(t, env.SiteBrowser(t), token, "grace-password-2")
	assert.IsType(t, &siteapi.SiteAuthVerifyEmailOK{}, again, "the scanner-style request consumed nothing")
}

func TestTheLinkInAnotherBrowserWithAPasswordMakesThatPasswordLiveAndDiscardsTheFormOne(t *testing.T) {
	env := testhelper.Setup(t)
	token := registerIn(t, env, env.SiteBrowser(t), "Grace Hopper", "grace@example.com", formPassword)

	res := verifyIn(t, env.SiteBrowser(t), token, "grace-password-2")

	session := sessionOf(t, res)
	landsInFirstWorkspace(t, env, session, "grace-hopper")
	assert.Equal(t, http.StatusOK, loginStatus(t, env, "grace@example.com", "grace-password-2"))
	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, "grace@example.com", formPassword))
}

// Pre-hijacking: the attacker registers the victim's address with a password they know;
// the victim follows the mail in their own browser and sets their own.
func TestAnAttackersPasswordNeverGoesLiveWhenTheVictimFollowsTheLink(t *testing.T) {
	env := testhelper.Setup(t)
	attacker := env.SiteBrowser(t)
	token := registerIn(t, env, attacker, "Victim", "victim@example.com", "attacker-password-1")
	victim := env.SiteBrowser(t)

	assert.IsType(t, &siteapi.SiteAuthVerifyEmailPreconditionRequired{}, verifyIn(t, victim, token, ""))
	require.IsType(t, &siteapi.SiteAuthVerifyEmailOK{}, verifyIn(t, victim, token, "victim-password-1"))

	assert.Equal(t, http.StatusOK, loginStatus(t, env, "victim@example.com", "victim-password-1"))
	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, "victim@example.com", "attacker-password-1"))
}

func TestRegisteringTheSameUnverifiedAddressTwiceLeavesOnlyTheLatestMailAndPassword(t *testing.T) {
	env := testhelper.Setup(t)
	browser := env.SiteBrowser(t)
	first := registerIn(t, env, browser, "Grace Hopper", "grace@example.com", "first-password-1")
	latest := registerIn(t, env, browser, "Grace Hopper", "grace@example.com", "latest-password-1")

	assert.IsType(t, &siteapi.SiteAuthVerifyEmailBadRequest{}, verifyIn(t, browser, first, ""), "the earlier mail is dead")
	assert.IsType(t, &siteapi.SiteAuthVerifyEmailBadRequest{}, verifyIn(t, browser, first, "attacker-password-1"))
	require.IsType(t, &siteapi.SiteAuthVerifyEmailOK{}, verifyIn(t, browser, latest, ""))

	assert.Equal(t, http.StatusOK, loginStatus(t, env, "grace@example.com", "latest-password-1"))
	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, "grace@example.com", "first-password-1"))
}

// ADR 0031: a Verified address answers the same as a new one, cookie included, and its
// password is not stored.
func TestRegisteringAVerifiedAddressSetsTheSameCookieAndStoresNothing(t *testing.T) {
	env := testhelper.Setup(t)
	before, err := env.DB.User.Get(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	cookieOf := func(email string) *http.Cookie {
		res, err := env.SiteAnonymous(t).SiteAuthRegister(t.Context(), &siteapi.SiteRegisterInput{
			Name: "Mallory", Email: siteapi.EmailAddress(email), Password: siteapi.NewOptString("mallory-password-1"),
		})
		require.NoError(t, err)
		accepted, ok := res.(*siteapi.SiteAuthRegisterAccepted)
		require.Truef(t, ok, "got %T", res)
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

func TestResendAsksForAPasswordEvenInTheRegistrationBrowser(t *testing.T) {
	env := testhelper.Setup(t)
	browser := env.SiteBrowser(t)
	registerIn(t, env, browser, "Grace Hopper", "grace@example.com", formPassword)
	res, err := browser.SiteAuthResendVerification(t.Context(), &siteapi.SiteResendVerificationInput{Email: "grace@example.com"})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteAuthResendVerificationAccepted{}, res)
	_, body := lastSystemEmail(t, env)
	resent := tokenFromEmail(t, body)

	assert.IsType(t, &siteapi.SiteAuthVerifyEmailPreconditionRequired{}, verifyIn(t, browser, resent, ""))
	assert.IsType(t, &siteapi.SiteAuthVerifyEmailOK{}, verifyIn(t, browser, resent, "grace-password-2"))
}

func TestABrowsersCookieDoesNotVouchForAnotherRegistrationsLink(t *testing.T) {
	env := testhelper.Setup(t)
	browser := env.SiteBrowser(t)
	token := registerIn(t, env, browser, "Grace Hopper", "grace@example.com", formPassword)

	// The link of a registration made in another browser still needs a password.
	other := register(t, env, "Alan Turing", "alan@example.com")
	assert.IsType(t, &siteapi.SiteAuthVerifyEmailPreconditionRequired{}, verifyIn(t, browser, other, ""))
	assert.IsType(t, &siteapi.SiteAuthVerifyEmailOK{}, verifyIn(t, browser, token, ""))
}
