package site_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// requestLink asks for the deletion link and returns the token of the mail it sent.
func requestLink(t *testing.T, env *testhelper.TestEnv, c *siteapi.Client, email string) string {
	t.Helper()
	res, err := c.SiteUserDeleteUserLink(t.Context(), &siteapi.SiteDeleteUserLinkInput{Email: siteapi.EmailAddress(email)})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteUserDeleteUserLinkAccepted{}, res)
	_, body := lastSystemEmail(t, env)
	return tokenFromEmail(t, body)
}

func TestAProviderOnlyUserDeletesThemselfThroughTheMailedLink(t *testing.T) {
	env := testhelper.Setup(t)
	session := env.SiteToken(t, fixtures.ProviderOnlyPiaEmail, nil)
	pia := env.SiteActor(t, fixtures.ProviderOnlyPiaEmail)

	token := requestLink(t, env, pia, fixtures.ProviderOnlyPiaEmail)
	subject, _ := lastSystemEmail(t, env)
	assert.Contains(t, subject, "Confirm")
	assert.Equal(t, fixtures.ProviderOnlyPiaEmail, env.SystemMail.Messages()[0].To)

	res := deleteUser(t, pia, &siteapi.SiteDeleteUserInput{Email: fixtures.ProviderOnlyPiaEmail, Token: siteapi.NewOptString(token)})
	require.IsType(t, &siteapi.SiteUserDeleteUserNoContent{}, res)

	assert.False(t, userExists(t, env, fixtures.ProviderOnlyPiaEmail))
	assert.Equal(t, http.StatusUnauthorized, sendWithSession(t, env, http.MethodGet, "/site/me", session, "").Code)
	assert.Contains(t, env.SystemMail.Messages()[1].Subject, "deleted", "the notice follows the link mail")
}

func TestAProviderOnlyUserWithoutTheTokenIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	pia := env.SiteActor(t, fixtures.ProviderOnlyPiaEmail)

	res := deleteUser(t, pia, &siteapi.SiteDeleteUserInput{Email: fixtures.ProviderOnlyPiaEmail})

	assert.IsType(t, &siteapi.SiteUserDeleteUserBadRequest{}, res)
	assert.True(t, userExists(t, env, fixtures.ProviderOnlyPiaEmail))
}

func TestAProviderOnlyUserWithASecondFactorNeedsTheCodeToo(t *testing.T) {
	env := testhelper.Setup(t)
	pax := env.SiteActor(t, fixtures.ProviderOnlyPaxEmail)
	token := requestLink(t, env, pax, fixtures.ProviderOnlyPaxEmail)

	missing := deleteUser(t, pax, &siteapi.SiteDeleteUserInput{Email: fixtures.ProviderOnlyPaxEmail, Token: siteapi.NewOptString(token)})
	assert.IsType(t, &siteapi.SiteUserDeleteUserForbidden{}, missing)
	assert.True(t, userExists(t, env, fixtures.ProviderOnlyPaxEmail), "the link alone does not delete")

	code, err := totp.GenerateCode(fixtures.SecondFactorSamTotpSecret, time.Now())
	require.NoError(t, err)
	done := deleteUser(t, pax, &siteapi.SiteDeleteUserInput{
		Email: fixtures.ProviderOnlyPaxEmail, Token: siteapi.NewOptString(token), Code: siteapi.NewOptString(code),
	})
	assert.IsType(t, &siteapi.SiteUserDeleteUserNoContent{}, done, "a refused code leaves the link usable")
	assert.False(t, userExists(t, env, fixtures.ProviderOnlyPaxEmail))
}

func TestAUsedLinkIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	pia := env.SiteActor(t, fixtures.ProviderOnlyPiaEmail)
	token := requestLink(t, env, pia, fixtures.ProviderOnlyPiaEmail)
	deleteUser(t, pia, &siteapi.SiteDeleteUserInput{Email: fixtures.ProviderOnlyPiaEmail, Token: siteapi.NewOptString(token)})
	// The same address signs up again as a new User; the old link is bound to the old id.
	// test writes: production Sign-up cannot make a password-less User, the successor of a provider-only User under test; a provider sign-in flow is a far heavier setup than the one row
	require.NoError(t, env.DB.User.Create().SetName("Pia Again").SetEmail(fixtures.ProviderOnlyPiaEmail).SetEmailVerifiedAt(time.Now()).Exec(t.Context()))
	again := env.SiteActor(t, fixtures.ProviderOnlyPiaEmail)

	res := deleteUser(t, again, &siteapi.SiteDeleteUserInput{Email: fixtures.ProviderOnlyPiaEmail, Token: siteapi.NewOptString(token)})

	assert.IsType(t, &siteapi.SiteUserDeleteUserBadRequest{}, res, "a link issued to a deleted User cannot act on its successor")
	assert.True(t, userExists(t, env, fixtures.ProviderOnlyPiaEmail))
}

func TestAnExpiredLinkIsRefused(t *testing.T) {
	now := time.Now()
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return now }))
	pia := env.SiteActor(t, fixtures.ProviderOnlyPiaEmail)
	token := requestLink(t, env, pia, fixtures.ProviderOnlyPiaEmail)
	now = now.Add(time.Hour + time.Second)

	res := deleteUser(t, pia, &siteapi.SiteDeleteUserInput{Email: fixtures.ProviderOnlyPiaEmail, Token: siteapi.NewOptString(token)})

	assert.IsType(t, &siteapi.SiteUserDeleteUserBadRequest{}, res)
	assert.True(t, userExists(t, env, fixtures.ProviderOnlyPiaEmail))
}

func TestAnotherUsersLinkIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	token := requestLink(t, env, env.SiteActor(t, fixtures.ProviderOnlyPaxEmail), fixtures.ProviderOnlyPaxEmail)
	pia := env.SiteActor(t, fixtures.ProviderOnlyPiaEmail)

	res := deleteUser(t, pia, &siteapi.SiteDeleteUserInput{Email: fixtures.ProviderOnlyPiaEmail, Token: siteapi.NewOptString(token)})

	assert.IsType(t, &siteapi.SiteUserDeleteUserBadRequest{}, res)
	assert.True(t, userExists(t, env, fixtures.ProviderOnlyPiaEmail))
}

func TestALinkDiesWhenTheUserSignsOutEverywhere(t *testing.T) {
	env := testhelper.Setup(t)
	pia := env.SiteActor(t, fixtures.ProviderOnlyPiaEmail)
	token := requestLink(t, env, pia, fixtures.ProviderOnlyPiaEmail)
	require.NoError(t, env.DB.User.UpdateOneID(fixtures.ProviderOnlyPiaID).AddSessionEpoch(1).Exec(t.Context()))
	fresh := env.SiteActor(t, fixtures.ProviderOnlyPiaEmail)

	res := deleteUser(t, fresh, &siteapi.SiteDeleteUserInput{Email: fixtures.ProviderOnlyPiaEmail, Token: siteapi.NewOptString(token)})

	assert.IsType(t, &siteapi.SiteUserDeleteUserBadRequest{}, res)
}

func TestAWrongTypedEmailIsRefusedAtTheLinkAndAtTheDeletion(t *testing.T) {
	env := testhelper.Setup(t)
	pia := env.SiteActor(t, fixtures.ProviderOnlyPiaEmail)
	token := requestLink(t, env, pia, fixtures.ProviderOnlyPiaEmail)

	link, err := pia.SiteUserDeleteUserLink(t.Context(), &siteapi.SiteDeleteUserLinkInput{Email: fixtures.OwnerJohnEmail})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteDeleteUserProblemDetails{}, link)
	assert.Len(t, env.SystemMail.Messages(), 1, "no second mail")

	res := deleteUser(t, pia, &siteapi.SiteDeleteUserInput{Email: fixtures.OwnerJohnEmail, Token: siteapi.NewOptString(token)})
	assert.IsType(t, &siteapi.SiteDeleteUserProblemDetails{}, res)
	assert.True(t, userExists(t, env, fixtures.ProviderOnlyPiaEmail))
}

func TestAUserWithAPasswordGetsNoLink(t *testing.T) {
	env := testhelper.Setup(t)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)

	res, err := mary.SiteUserDeleteUserLink(t.Context(), &siteapi.SiteDeleteUserLinkInput{Email: fixtures.MemberMaryEmail})

	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteDeleteUserProblemDetails{}, res)
	assert.Empty(t, env.SystemMail.Messages())
}

func TestWrongCodesOfAProviderOnlyUserAreThrottledOnTheLoginBudget(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{LoginFailures: 3}))
	pax := env.SiteActor(t, fixtures.ProviderOnlyPaxEmail)
	token := requestLink(t, env, pax, fixtures.ProviderOnlyPaxEmail)
	input := func(code string) *siteapi.SiteDeleteUserInput {
		return &siteapi.SiteDeleteUserInput{
			Email: fixtures.ProviderOnlyPaxEmail, Token: siteapi.NewOptString(token), Code: siteapi.NewOptString(code),
		}
	}

	for range 3 {
		wrong := deleteUser(t, pax, input("000000"))
		require.IsType(t, &siteapi.SiteUserDeleteUserForbidden{}, wrong)
	}
	right, err := totp.GenerateCode(fixtures.ProviderOnlyPaxTotpSecret, time.Now())
	require.NoError(t, err)

	res := deleteUser(t, pax, input(right))

	assert.IsType(t, &siteapi.ProblemDetailsHeaders{}, res, "three wrong codes took the delay, even the right code waits")
	assert.True(t, userExists(t, env, fixtures.ProviderOnlyPaxEmail))
}

func TestAProviderOnlySoleOwnerOfALiveWorkspaceIsRefusedEvenWithTheLink(t *testing.T) {
	env := testhelper.Setup(t)
	pru := env.SiteActor(t, fixtures.ProviderOnlyOwnerPruEmail)
	token := requestLink(t, env, pru, fixtures.ProviderOnlyOwnerPruEmail)

	res := deleteUser(t, pru, &siteapi.SiteDeleteUserInput{Email: fixtures.ProviderOnlyOwnerPruEmail, Token: siteapi.NewOptString(token)})

	refused, ok := res.(*siteapi.SiteDeleteUserProblemDetails)
	require.Truef(t, ok, "got %T", res)
	listed := lo.Map(refused.Workspaces, func(w siteapi.SiteSoleOwnedWorkspace, _ int) string { return w.Name })
	assert.Equal(t, []string{fixtures.CyberdyneName}, listed)
	assert.True(t, userExists(t, env, fixtures.ProviderOnlyOwnerPruEmail))
	assert.Equal(t, []siteapi.WorkspaceId{fixtures.CyberdynePublicID}, workspaceIDs(t, pru))
}
