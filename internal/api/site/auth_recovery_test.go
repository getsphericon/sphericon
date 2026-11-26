package site_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// tokenFromEmail extracts the token query param from an account email's link.
func tokenFromEmail(t *testing.T, body string) string {
	t.Helper()
	idx := strings.Index(body, "token=")
	require.GreaterOrEqualf(t, idx, 0, "email should carry a token link: %q", body)
	rest := body[idx+len("token="):]
	if end := strings.IndexAny(rest, " \n\r\t"); end >= 0 {
		rest = rest[:end]
	}
	tok, err := url.QueryUnescape(rest)
	require.NoError(t, err)
	return tok
}

// lastSystemEmail returns the most recent captured platform email.
func lastSystemEmail(t *testing.T, env *testhelper.TestEnv) (subject, body string) {
	t.Helper()
	msgs := env.SystemMail.Messages()
	require.NotEmpty(t, msgs, "expected a system email to be sent")
	last := msgs[len(msgs)-1]
	return last.Subject, last.Text
}

func TestForgotPasswordFullResetFlow(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)
	ctx := t.Context()

	err := c.SiteAuthForgotPassword(ctx, &siteapi.SiteForgotPasswordInput{Email: fixtures.OwnerJohnEmail})
	require.NoError(t, err)

	_, body := lastSystemEmail(t, env)
	token := tokenFromEmail(t, body)

	err = c.SiteAuthResetPassword(ctx, &siteapi.SiteResetPasswordInput{Token: token, Password: "brandnewpass1"})
	require.NoError(t, err)

	// The new password logs in; the old one no longer does.
	assert.Equal(t, http.StatusOK, loginStatus(t, env, fixtures.OwnerJohnEmail, "brandnewpass1"))
	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword))

	// The token is single-use: replaying it after the reset fails (the binding —
	// the password hash — has changed).
	err = c.SiteAuthResetPassword(ctx, &siteapi.SiteResetPasswordInput{Token: token, Password: "another123"})
	testhelper.SiteRefusal(t, err, http.StatusBadRequest, "reset token must not be reusable")
}

func TestForgotPasswordUnknownEmailIsSilent(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)

	err := c.SiteAuthForgotPassword(t.Context(),
		&siteapi.SiteForgotPasswordInput{Email: "nobody@example.com"})
	require.NoError(t, err)
	assert.Empty(t, env.SystemMail.Messages(), "no email for an unknown address (no enumeration)")
}

func TestResetPasswordRejectsInvalidToken(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)

	err := c.SiteAuthResetPassword(t.Context(),
		&siteapi.SiteResetPasswordInput{Token: "not-a-token", Password: "whatever123"})
	testhelper.SiteRefusal(t, err, http.StatusBadRequest)
}

func TestResetPasswordRejectsWrongPurposeToken(t *testing.T) {
	env := testhelper.Setup(t)
	pub := env.SiteAnonymous(t)
	ctx := t.Context()

	// Take a verify token from a Sign-up mail, then try to use it as a reset token.
	verifyToken := signUpLink(t, env, "Grace Hopper", "grace@example.com")

	err := pub.SiteAuthResetPassword(ctx, &siteapi.SiteResetPasswordInput{Token: verifyToken, Password: "whatever123"})
	testhelper.SiteRefusal(t, err, http.StatusBadRequest, "a verify token must not reset a password")
}

func TestEmailChangeRequiresCurrentPassword(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	err := c.SiteUserEmailChange(t.Context(), &siteapi.SiteEmailChangeInput{
		NewEmail:        "new@example.com",
		CurrentPassword: "wrong",
	})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)
	assert.Empty(t, env.SystemMail.Messages())
}

// ADR 0031: asking for a Taken address answers 202 like a free one; the owner gets a
// tokenless notice and the requester no confirm link.
func TestEmailChangeToATakenAddressAnswersLikeAFreeOneAndNoticesItsOwner(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	for _, target := range []string{"free@example.com", fixtures.MemberMaryEmail} {
		err := c.SiteUserEmailChange(ctx, &siteapi.SiteEmailChangeInput{
			NewEmail:        siteapi.EmailAddress(target),
			CurrentPassword: fixtures.OwnerJohnPassword,
		})
		require.NoError(t, err, target)
	}

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 2, "one mail job per request")
	assert.Equal(t, "free@example.com", msgs[0].To)
	assert.Contains(t, msgs[0].Text, "token=")
	assert.Equal(t, fixtures.MemberMaryEmail, msgs[1].To)
	assert.NotContains(t, msgs[1].Text, "token=", "a notice must never carry a token")
}

func TestEmailChangeStopsMailingAnAddressOnceItsBudgetIsSpent(t *testing.T) {
	const limit = 3
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{ForgotAddress: limit}))
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	for range limit + 2 {
		err := c.SiteUserEmailChange(t.Context(), &siteapi.SiteEmailChangeInput{
			NewEmail:        fixtures.MemberMaryEmail,
			CurrentPassword: fixtures.OwnerJohnPassword,
		})
		require.NoError(t, err)
	}

	assert.Len(t, env.SystemMail.Messages(), limit)
}

// A password change or signing out everywhere ends what the sessions before it asked for: an email
// change link mailed before it no longer confirms, and the address stays.
func TestEmailChangeLinkDiesWithTheSessionsThatAskedForIt(t *testing.T) {
	cases := map[string]func(t *testing.T, c *siteapi.Client){
		"signing out everywhere": func(t *testing.T, c *siteapi.Client) {
			t.Helper()
			_, err := c.SiteUserSignOutEverywhere(t.Context())
			require.NoError(t, err)
		},
		"a password change": func(t *testing.T, c *siteapi.Client) {
			t.Helper()
			_, err := c.SiteUserUpdateMe(t.Context(), &siteapi.SiteUpdateMeInput{
				CurrentPassword: siteapi.NewOptString(fixtures.OwnerJohnPassword), NewPassword: siteapi.NewOptString("a-brand-new-password-1"),
			})
			require.NoError(t, err)
		},
	}
	for name, revoke := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			authed := env.SiteActor(t, fixtures.OwnerJohnEmail)
			err := authed.SiteUserEmailChange(t.Context(), &siteapi.SiteEmailChangeInput{
				NewEmail: "moved@example.com", CurrentPassword: fixtures.OwnerJohnPassword,
			})
			require.NoError(t, err)
			_, body := lastSystemEmail(t, env)
			revoke(t, authed)

			_, err = env.SiteAnonymous(t).SiteAuthConfirmEmailChange(t.Context(),
				&siteapi.SiteConfirmEmailChangeInput{Token: tokenFromEmail(t, body)}, siteapi.SiteAuthConfirmEmailChangeParams{})

			refused := testhelper.SiteRefusal(t, err, http.StatusBadRequest)
			assert.Equal(t, "invalid or expired confirmation link", refused.Detail.Value)
			assert.Equal(t, fixtures.OwnerJohnEmail, userByEmail(t, env, fixtures.OwnerJohnEmail).Email)
		})
	}
}

func TestEmailChangeConfirmSwapsEmail(t *testing.T) {
	env := testhelper.Setup(t)
	authed := env.SiteActor(t, fixtures.OwnerJohnEmail)
	pub := env.SiteAnonymous(t)
	ctx := t.Context()

	err := authed.SiteUserEmailChange(ctx, &siteapi.SiteEmailChangeInput{
		NewEmail:        "moved@example.com",
		CurrentPassword: fixtures.OwnerJohnPassword,
	})
	require.NoError(t, err)

	// The confirmation link is sent to the NEW address.
	subject, body := lastSystemEmail(t, env)
	assert.Contains(t, subject, "new")
	token := tokenFromEmail(t, body)

	_, err = pub.SiteAuthConfirmEmailChange(ctx, &siteapi.SiteConfirmEmailChangeInput{Token: token}, siteapi.SiteAuthConfirmEmailChangeParams{})
	require.NoError(t, err)

	// Login now works with the new email, not the old one.
	assert.Equal(t, http.StatusOK, loginStatus(t, env, "moved@example.com", fixtures.OwnerJohnPassword))
	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword))
}
