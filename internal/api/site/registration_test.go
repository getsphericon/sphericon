package site_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entmembership "github.com/getsphericon/sphericon/ent/membership"
	entuser "github.com/getsphericon/sphericon/ent/user"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// register drives the public registration and returns the token carried by the
// verification email it sent.
func register(t *testing.T, env *testhelper.TestEnv, name, email string) string {
	t.Helper()
	res, err := env.SiteAnonymous(t).SiteAuthRegister(t.Context(),
		&siteapi.SiteRegisterInput{Name: name, Email: siteapi.EmailAddress(email)})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteAuthRegisterAccepted{}, res)
	_, body := lastSystemEmail(t, env)
	return tokenFromEmail(t, body)
}

// verify follows the verification link: it proves the address and sets the password.
func verify(t *testing.T, env *testhelper.TestEnv, token, password string) siteapi.SiteAuthVerifyEmailRes {
	t.Helper()
	res, err := env.SiteAnonymous(t).SiteAuthVerifyEmail(t.Context(),
		&siteapi.SiteVerifyEmailInput{Token: token, Password: password})
	require.NoError(t, err)
	return res
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
	assert.Empty(t, u.PasswordHash, "no password exists before the address is proven")
	assert.Nil(t, u.EmailVerifiedAt)
	memberships, err := env.DB.Membership.Query().Where(entmembership.UserID(u.ID)).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, memberships, "no Workspace yet")
}

func TestRegisteringValidatesNameAndEmailAndRefusesATakenAddress(t *testing.T) {
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

	dup, err := c.SiteAuthRegister(t.Context(), &siteapi.SiteRegisterInput{Name: "John again", Email: fixtures.OwnerJohnEmail})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAuthRegisterConflict{}, dup)
	assert.Empty(t, env.SystemMail.Messages())
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

func TestTheVerifyLinkWorksOnce(t *testing.T) {
	env := testhelper.Setup(t)
	token := register(t, env, "Grace Hopper", "grace@example.com")
	require.IsType(t, &siteapi.SiteAuthVerifyEmailOK{}, verify(t, env, token, "grace-password-1"))

	assert.IsType(t, &siteapi.ProblemDetails{}, verify(t, env, token, "attacker-password-1"), "a replayed link sets nothing")

	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, "grace@example.com", "attacker-password-1"))
	assert.Equal(t, http.StatusOK, loginStatus(t, env, "grace@example.com", "grace-password-1"))
}

func TestTheVerifyLinkRefusesBadInput(t *testing.T) {
	env := testhelper.Setup(t)
	token := register(t, env, "Grace Hopper", "grace@example.com")

	assert.IsType(t, &siteapi.ProblemDetails{}, verify(t, env, token, ""), "a password is required")
	assert.IsType(t, &siteapi.ProblemDetails{}, verify(t, env, "garbage", "grace-password-1"))
	assert.Equal(t, http.StatusForbidden, loginStatus(t, env, "grace@example.com", "grace-password-1"),
		"a refused link leaves the User unverified")
}

func TestAnOldLinkNeverVerifiesAChangedAddress(t *testing.T) {
	env := testhelper.Setup(t)
	token := register(t, env, "Grace Hopper", "grace@example.com")
	u, err := env.DB.User.Query().Where(entuser.Email("grace@example.com")).Only(t.Context())
	require.NoError(t, err)
	require.NoError(t, env.DB.User.UpdateOneID(u.ID).SetEmail("grace@elsewhere.example").Exec(t.Context()))

	assert.IsType(t, &siteapi.ProblemDetails{}, verify(t, env, token, "grace-password-1"))

	after, err := env.DB.User.Get(t.Context(), u.ID)
	require.NoError(t, err)
	assert.Nil(t, after.EmailVerifiedAt)
	assert.Empty(t, after.PasswordHash)
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
