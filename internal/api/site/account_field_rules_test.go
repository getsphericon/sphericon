package site_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Every way a person sets a password, a name or an address of their own goes through the same
// rules as Sign-up (internal/validation), so none of them is a way around it.

const (
	shortPasswordMessage = "the password must be 8 to 256 characters long"
	badNameMessage       = "the name must be at most 100 characters on one line, with at least one visible character and no control or invisible ones"
)

func TestSigningUpWithANameThatBreaksALineIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.SiteAnonymous(t).SiteAuthSignUp(t.Context(), &siteapi.SiteSignUpInput{
		Name: "Grace\r\nBcc: all@example.com", Email: "grace@example.com", Password: siteapi.NewOptString("grace-password-1"),
	})
	problem := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, siteapi.ProblemDetailsErrors{"name": {badNameMessage}}, problem.Errors.Value)
	assert.Empty(t, env.SystemMail.Messages())
}

func TestSigningUpATakenAddressWithABadNameIsAnsweredAsANewOne(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.SiteAnonymous(t).SiteAuthSignUp(t.Context(), &siteapi.SiteSignUpInput{
		Name: "John\u2028Smith", Email: fixtures.OwnerJohnEmail, Password: siteapi.NewOptString("john-password-12"),
	})
	problem := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, siteapi.ProblemDetailsErrors{"name": {badNameMessage}}, problem.Errors.Value)
	assert.Empty(t, env.SystemMail.Messages(), "nothing says the address is taken")
}

func TestAcceptingAnInvitationWithATooLongNameIsRefusedByTheContract(t *testing.T) {
	env := testhelper.Setup(t)
	token := fixtures.InvitationAcmeNewcomerToken

	err := env.SiteAnonymous(t).SitePublicInvitationsAccept(t.Context(),
		&siteapi.SiteAcceptInvitationInput{Name: siteapi.NewOptUserName(siteapi.UserName(strings.Repeat("a", 101))), Password: siteapi.NewOptString(formPassword)},
		siteapi.SitePublicInvitationsAcceptParams{Token: token})

	testhelper.SiteRefusal(t, err, http.StatusBadRequest)
	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, fixtures.InvitationAcmeNewcomerEmail, formPassword), "no account was made")
}

func TestAcceptingAnInvitationWithAnInvisibleNameIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	token := fixtures.InvitationAcmeNewcomerToken

	err := env.SiteAnonymous(t).SitePublicInvitationsAccept(t.Context(),
		&siteapi.SiteAcceptInvitationInput{Name: siteapi.NewOptUserName("\u200b\u200d"), Password: siteapi.NewOptString(formPassword)},
		siteapi.SitePublicInvitationsAcceptParams{Token: token})
	problem := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, siteapi.ProblemDetailsErrors{"name": {badNameMessage}}, problem.Errors.Value)
	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, fixtures.InvitationAcmeNewcomerEmail, formPassword), "no account was made")
}

func TestRenamingToANameWithABidiOverrideIsRefusedAndKeepsTheOldOne(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	before, err := c.SiteUserGetMe(t.Context())
	require.NoError(t, err)

	_, err = c.SiteUserUpdateMe(t.Context(), &siteapi.SiteUpdateMeInput{Name: siteapi.NewOptUserName("John \u202eetisoppo")})
	problem := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, siteapi.ProblemDetailsErrors{"name": {badNameMessage}}, problem.Errors.Value)
	after, err := c.SiteUserGetMe(t.Context())
	require.NoError(t, err)
	assert.Equal(t, before.Name, after.Name)
}

func TestSigningUpWithAPasswordOfSpacesIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.SiteAnonymous(t).SiteAuthSignUp(t.Context(), &siteapi.SiteSignUpInput{
		Name: "Grace Hopper", Email: "grace@example.com", Password: siteapi.NewOptString("        "),
	})
	problem := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, siteapi.ProblemDetailsErrors{"password": {"the password must not be only spaces"}}, problem.Errors.Value)
}

func TestResettingToAShortPasswordIsRefusedAndKeepsTheLink(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)
	err := c.SiteAuthForgotPassword(t.Context(), &siteapi.SiteForgotPasswordInput{Email: fixtures.OwnerJohnEmail})
	require.NoError(t, err)
	_, body := lastSystemEmail(t, env)
	token := tokenFromEmail(t, body)

	err = c.SiteAuthResetPassword(t.Context(), &siteapi.SiteResetPasswordInput{Token: token, Password: "1"})
	problem := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, siteapi.ProblemDetailsErrors{"password": {shortPasswordMessage}}, problem.Errors.Value)
	assert.Equal(t, shortPasswordMessage, problem.Detail.Value, "a single failure is the detail the SPA shows")
	assert.Equal(t, http.StatusOK, loginStatus(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword))

	err = c.SiteAuthResetPassword(t.Context(), &siteapi.SiteResetPasswordInput{Token: token, Password: "brandnewpass1"})
	assert.NoError(t, err, "the refused request spent nothing")
}

func TestTheLinkInAnotherBrowserRefusesAShortPasswordAndStaysValid(t *testing.T) {
	env := testhelper.Setup(t)
	token := signUpIn(t, env, env.SiteBrowser(t), "Grace Hopper", "grace@example.com", formPassword)

	_, err := env.SiteBrowser(t).SiteAuthVerifyEmail(t.Context(),
		&siteapi.SiteVerifyEmailInput{Token: token, Password: siteapi.NewOptString("1")}, siteapi.SiteAuthVerifyEmailParams{})
	problem := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, siteapi.ProblemDetailsErrors{"password": {shortPasswordMessage}}, problem.Errors.Value)
	assert.Nil(t, userByEmail(t, env, "grace@example.com").EmailVerifiedAt)

	_, err = env.SiteBrowser(t).SiteAuthVerifyEmail(t.Context(),
		&siteapi.SiteVerifyEmailInput{Token: token, Password: siteapi.NewOptString("grace-password-2")}, siteapi.SiteAuthVerifyEmailParams{})
	assert.NoError(t, err)
}

func TestAcceptingAnInvitationWithAShortPasswordIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	token := fixtures.InvitationAcmeNewcomerToken

	err := env.SiteAnonymous(t).SitePublicInvitationsAccept(t.Context(),
		&siteapi.SiteAcceptInvitationInput{Name: siteapi.NewOptUserName("Newcomer"), Password: siteapi.NewOptString("1")},
		siteapi.SitePublicInvitationsAcceptParams{Token: token})
	problem := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, siteapi.ProblemDetailsErrors{"password": {shortPasswordMessage}}, problem.Errors.Value)
	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, "newcomer@example.com", "1"), "no account was made")
}

func TestChangingToAShortPasswordIsRefusedAndKeepsTheOldOne(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteUserUpdateMe(t.Context(), &siteapi.SiteUpdateMeInput{
		CurrentPassword: siteapi.NewOptString(fixtures.OwnerJohnPassword), NewPassword: siteapi.NewOptString("1"),
	})
	problem := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, siteapi.ProblemDetailsErrors{"newPassword": {shortPasswordMessage}}, problem.Errors.Value)
	assert.Equal(t, http.StatusOK, loginStatus(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword))
}

func TestInvitingAnUnreachableAddressIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteInvitationsCreate(t.Context(),
		&siteapi.SiteCreateInvitationInput{Email: "newcomer@example", Role: siteapi.SiteInvitableRoleAdmin},
		siteapi.SiteInvitationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	problem := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, problem.Errors.Value, "email")
	assert.Empty(t, env.SystemMail.Messages())
}

func TestChangingTheEmailToAnUnreachableAddressIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteUserEmailChange(t.Context(), &siteapi.SiteEmailChangeInput{
		NewEmail: "john@example", CurrentPassword: fixtures.OwnerJohnPassword,
	})
	problem := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, problem.Errors.Value, "newEmail")
	assert.Empty(t, env.SystemMail.Messages())
}
