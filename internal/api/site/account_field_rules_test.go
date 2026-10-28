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
// rules as registration (internal/validation), so none of them is a way around it.

const (
	shortPasswordMessage = "the password must be 8 to 256 characters long"
	badNameMessage       = "the name must be at most 100 characters on one line, with at least one visible character and no control or invisible ones"
)

func TestRegisteringWithANameThatBreaksALineIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	res := registerWithPassword(t, env, "Grace\r\nBcc: all@example.com", "grace@example.com", "grace-password-1")
	problem, ok := res.(*siteapi.SiteAuthRegisterUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.ProblemDetailsErrors{"name": {badNameMessage}}, problem.Errors.Value)
	assert.Empty(t, env.SystemMail.Messages())
}

func TestRegisteringATakenAddressWithABadNameIsAnsweredAsANewOne(t *testing.T) {
	env := testhelper.Setup(t)

	res := registerWithPassword(t, env, "John\u2028Smith", fixtures.OwnerJohnEmail, "john-password-12")
	problem, ok := res.(*siteapi.SiteAuthRegisterUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.ProblemDetailsErrors{"name": {badNameMessage}}, problem.Errors.Value)
	assert.Empty(t, env.SystemMail.Messages(), "nothing says the address is taken")
}

func TestAcceptingAnInvitationWithATooLongNameIsRefusedByTheContract(t *testing.T) {
	env := testhelper.Setup(t)
	token := inviteToAcme(t, env, "newuser@example.com")

	_, err := env.SiteAnonymous(t).SitePublicInvitationsAccept(t.Context(),
		&siteapi.SiteAcceptInvitationInput{Name: siteapi.NewOptUserName(siteapi.UserName(strings.Repeat("a", 101))), Password: siteapi.NewOptString(formPassword)},
		siteapi.SitePublicInvitationsAcceptParams{Token: token})

	require.Error(t, err)
	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, "newuser@example.com", formPassword), "no account was made")
}

func TestAcceptingAnInvitationWithAnInvisibleNameIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	token := inviteToAcme(t, env, "newuser@example.com")

	res, err := env.SiteAnonymous(t).SitePublicInvitationsAccept(t.Context(),
		&siteapi.SiteAcceptInvitationInput{Name: siteapi.NewOptUserName("\u200b\u200d"), Password: siteapi.NewOptString(formPassword)},
		siteapi.SitePublicInvitationsAcceptParams{Token: token})
	require.NoError(t, err)
	problem, ok := res.(*siteapi.SitePublicInvitationsAcceptUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.ProblemDetailsErrors{"name": {badNameMessage}}, problem.Errors.Value)
	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, "newuser@example.com", formPassword), "no account was made")
}

func TestRenamingToANameWithABidiOverrideIsRefusedAndKeepsTheOldOne(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	before, err := c.SiteUserGetMe(t.Context())
	require.NoError(t, err)

	res, err := c.SiteUserUpdateMe(t.Context(), &siteapi.SiteUpdateMeInput{Name: siteapi.NewOptUserName("John \u202eetisoppo")})
	require.NoError(t, err)
	problem, ok := res.(*siteapi.SiteUserUpdateMeUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.ProblemDetailsErrors{"name": {badNameMessage}}, problem.Errors.Value)
	after, err := c.SiteUserGetMe(t.Context())
	require.NoError(t, err)
	assert.Equal(t, before.Name, after.Name)
}

func TestRegisteringWithAPasswordOfSpacesIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	res := registerWithPassword(t, env, "Grace Hopper", "grace@example.com", "        ")
	problem, ok := res.(*siteapi.SiteAuthRegisterUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.ProblemDetailsErrors{"password": {"the password must not be only spaces"}}, problem.Errors.Value)
}

func TestResettingToAShortPasswordIsRefusedAndKeepsTheLink(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)
	_, err := c.SiteAuthForgotPassword(t.Context(), &siteapi.SiteForgotPasswordInput{Email: fixtures.OwnerJohnEmail})
	require.NoError(t, err)
	_, body := lastSystemEmail(t, env)
	token := tokenFromEmail(t, body)

	res, err := c.SiteAuthResetPassword(t.Context(), &siteapi.SiteResetPasswordInput{Token: token, Password: "1"})
	require.NoError(t, err)
	problem, ok := res.(*siteapi.SiteAuthResetPasswordUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.ProblemDetailsErrors{"password": {shortPasswordMessage}}, problem.Errors.Value)
	assert.Equal(t, shortPasswordMessage, problem.Detail.Value, "a single failure is the detail the SPA shows")
	assert.Equal(t, http.StatusOK, loginStatus(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword))

	again, err := c.SiteAuthResetPassword(t.Context(), &siteapi.SiteResetPasswordInput{Token: token, Password: "brandnewpass1"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAuthResetPasswordOK{}, again, "the refused request spent nothing")
}

func TestTheLinkInAnotherBrowserRefusesAShortPasswordAndStaysValid(t *testing.T) {
	env := testhelper.Setup(t)
	token := registerIn(t, env, env.SiteBrowser(t), "Grace Hopper", "grace@example.com", formPassword)

	res := verifyIn(t, env.SiteBrowser(t), token, "1")
	problem, ok := res.(*siteapi.SiteAuthVerifyEmailUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.ProblemDetailsErrors{"password": {shortPasswordMessage}}, problem.Errors.Value)
	assert.Nil(t, userByEmail(t, env, "grace@example.com").EmailVerifiedAt)

	assert.IsType(t, &siteapi.SiteVerifyEmailResultHeaders{}, verifyIn(t, env.SiteBrowser(t), token, "grace-password-2"))
}

func TestAcceptingAnInvitationWithAShortPasswordIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	token := inviteToAcme(t, env, "newcomer@example.com")

	res, err := env.SiteAnonymous(t).SitePublicInvitationsAccept(t.Context(),
		&siteapi.SiteAcceptInvitationInput{Name: siteapi.NewOptUserName("Newcomer"), Password: siteapi.NewOptString("1")},
		siteapi.SitePublicInvitationsAcceptParams{Token: token})
	require.NoError(t, err)
	problem, ok := res.(*siteapi.SitePublicInvitationsAcceptUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.ProblemDetailsErrors{"password": {shortPasswordMessage}}, problem.Errors.Value)
	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, "newcomer@example.com", "1"), "no account was made")
}

func TestChangingToAShortPasswordIsRefusedAndKeepsTheOldOne(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteUserUpdateMe(t.Context(), &siteapi.SiteUpdateMeInput{
		CurrentPassword: siteapi.NewOptString(fixtures.OwnerJohnPassword), NewPassword: siteapi.NewOptString("1"),
	})
	require.NoError(t, err)
	problem, ok := res.(*siteapi.SiteUserUpdateMeUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.ProblemDetailsErrors{"newPassword": {shortPasswordMessage}}, problem.Errors.Value)
	assert.Equal(t, http.StatusOK, loginStatus(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword))
}

func TestInvitingAnUnreachableAddressIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteInvitationsCreate(t.Context(),
		&siteapi.SiteCreateInvitationInput{Email: "newcomer@example", Role: siteapi.SiteInvitableRoleAdmin},
		siteapi.SiteInvitationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	problem, ok := res.(*siteapi.SiteInvitationsCreateUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, problem.Errors.Value, "email")
	assert.Empty(t, env.SystemMail.Messages())
}

func TestChangingTheEmailToAnUnreachableAddressIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteUserEmailChange(t.Context(), &siteapi.SiteEmailChangeInput{
		NewEmail: "john@example", CurrentPassword: fixtures.OwnerJohnPassword,
	})
	require.NoError(t, err)
	problem, ok := res.(*siteapi.SiteUserEmailChangeUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, problem.Errors.Value, "newEmail")
	assert.Empty(t, env.SystemMail.Messages())
}
