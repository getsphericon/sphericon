package site_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/membership"
	entuser "github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/ent/workspace"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// signer is the production token signer keyed with the test instance's secret,
// for minting tokens in states the public flows never emit (unknown user, stale
// address, taken address).
func signer(t *testing.T) *authtoken.Signer {
	t.Helper()
	cfg, err := config.Load("test")
	require.NoError(t, err)
	return authtoken.New(cfg.JWTSecret)
}

func TestSiteAuthRegisterRejectsBadInput(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)
	ctx := context.Background()

	blank, err := c.SiteAuthRegister(ctx, &siteapi.SiteRegisterInput{Name: "  ", Email: "blank@example.com", Password: ""})
	require.NoError(t, err)
	problem, ok := blank.(*siteapi.SiteAuthRegisterUnprocessableEntity)
	require.Truef(t, ok, "got %T", blank)
	assert.Contains(t, problem.Errors.Value, "name")
	assert.Contains(t, problem.Errors.Value, "password")
	assert.NotContains(t, problem.Errors.Value, "email")
	exists, err := env.DB.User.Query().Where(entuser.Email("blank@example.com")).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, exists, "a rejected registration creates no user")

	dup, err := c.SiteAuthRegister(ctx, &siteapi.SiteRegisterInput{Name: "John again", Email: fixtures.OwnerJohnEmail, Password: "password123"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAuthRegisterConflict{}, dup)
}

// Registering creates a default workspace whose slug is derived from the name:
// a taken slug gets a numeric suffix and an unsluggable name falls back to "workspace".
func TestSiteAuthRegisterDerivesUniqueWorkspaceSlug(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)
	ctx := context.Background()

	register := func(name, email string) int64 {
		res, err := c.SiteAuthRegister(ctx, &siteapi.SiteRegisterInput{Name: name, Email: siteapi.EmailAddress(email), Password: "password123"})
		require.NoError(t, err)
		reg, ok := res.(*siteapi.SiteRegisterResult)
		require.Truef(t, ok, "got %T", res)
		return mustID(t, reg.ID)
	}

	taken := register("Acme", "acme2@example.com") // "acme" is a fixture workspace
	ws, err := env.DB.Workspace.Query().Where(workspace.HasMembershipsWith(membership.UserID(taken))).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, "acme-2", ws.Slug)
	role, err := env.DB.Membership.Query().Where(membership.UserID(taken), membership.WorkspaceID(ws.ID)).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, membership.RoleOwner, role.Role)

	bare := register("!!!", "bare@example.com")
	bareWS, err := env.DB.Workspace.Query().Where(workspace.HasMembershipsWith(membership.UserID(bare))).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, "workspace", bareWS.Slug)

	again := register("!!!", "bare2@example.com")
	againWS, err := env.DB.Workspace.Query().Where(workspace.HasMembershipsWith(membership.UserID(again))).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, "workspace-2", againWS.Slug)
}

func TestSiteAuthRecoveryRejectsBadTokens(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)
	ctx := context.Background()
	s := signer(t)
	john, err := env.DB.User.Get(ctx, fixtures.OwnerJohnID)
	require.NoError(t, err)

	t.Run("reset requires a password", func(t *testing.T) {
		res, err := c.SiteAuthResetPassword(ctx, &siteapi.SiteResetPasswordInput{Token: "whatever", Password: ""})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.ProblemDetails{}, res)
	})

	t.Run("verify rejects garbage", func(t *testing.T) {
		res, err := c.SiteAuthVerifyEmail(ctx, &siteapi.SiteVerifyEmailInput{Token: "garbage"})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.ProblemDetails{}, res)
	})

	t.Run("verify rejects a token for a user that no longer exists", func(t *testing.T) {
		tok, err := s.Mint(authtoken.PurposeEmailVerify, 987654, "", time.Hour, map[string]string{"email": "ghost@example.com"})
		require.NoError(t, err)
		res, err := c.SiteAuthVerifyEmail(ctx, &siteapi.SiteVerifyEmailInput{Token: tok})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.ProblemDetails{}, res)
	})

	t.Run("verify rejects a link for a previous address", func(t *testing.T) {
		tok, err := s.Mint(authtoken.PurposeEmailVerify, john.ID, "", time.Hour, map[string]string{"email": "old-address@example.com"})
		require.NoError(t, err)
		res, err := c.SiteAuthVerifyEmail(ctx, &siteapi.SiteVerifyEmailInput{Token: tok})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.ProblemDetails{}, res)
		after, err := env.DB.User.Get(ctx, john.ID)
		require.NoError(t, err)
		assert.Nil(t, after.EmailVerifiedAt, "a stale link must not verify the current address")
	})

	t.Run("confirm email change rejects garbage", func(t *testing.T) {
		res, err := c.SiteAuthConfirmEmailChange(ctx, &siteapi.SiteConfirmEmailChangeInput{Token: "garbage"}, siteapi.SiteAuthConfirmEmailChangeParams{})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteAuthConfirmEmailChangeBadRequest{}, res)
	})

	t.Run("confirm email change rejects a token without a new address", func(t *testing.T) {
		tok, err := s.Mint(authtoken.PurposeEmailChange, john.ID, john.Email, time.Hour, nil)
		require.NoError(t, err)
		res, err := c.SiteAuthConfirmEmailChange(ctx, &siteapi.SiteConfirmEmailChangeInput{Token: tok}, siteapi.SiteAuthConfirmEmailChangeParams{})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteAuthConfirmEmailChangeBadRequest{}, res)
	})

	t.Run("confirm email change conflicts when the address was taken meanwhile", func(t *testing.T) {
		tok, err := s.Mint(authtoken.PurposeEmailChange, john.ID, john.Email, time.Hour, map[string]string{"new": fixtures.MemberMaryEmail})
		require.NoError(t, err)
		res, err := c.SiteAuthConfirmEmailChange(ctx, &siteapi.SiteConfirmEmailChangeInput{Token: tok}, siteapi.SiteAuthConfirmEmailChangeParams{})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteAuthConfirmEmailChangeConflict{}, res)
		after, err := env.DB.User.Get(ctx, john.ID)
		require.NoError(t, err)
		assert.Equal(t, fixtures.OwnerJohnEmail, after.Email)
	})
}

func TestSiteUserEmailChangeAndResendEdgeCases(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	same, err := c.SiteUserEmailChange(ctx, &siteapi.SiteEmailChangeInput{
		NewEmail: siteapi.EmailAddress(fixtures.OwnerJohnEmail), CurrentPassword: fixtures.OwnerJohnPassword,
	})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteUserEmailChangeUnprocessableEntity{}, same, "the new address must differ")
	assert.Empty(t, env.SystemMail.Messages())

	anon := env.SiteAnonymous(t)
	_, err = anon.SiteUserEmailChange(ctx, &siteapi.SiteEmailChangeInput{NewEmail: "x@example.com", CurrentPassword: "x"})
	require.Error(t, err)
	require.Error(t, anon.SiteUserResendVerification(ctx))

	// Resend is a silent no-op once the address is verified.
	require.NoError(t, env.DB.User.UpdateOneID(fixtures.OwnerJohnID).SetEmailVerifiedAt(time.Now()).Exec(ctx))
	require.NoError(t, c.SiteUserResendVerification(ctx))
	assert.Empty(t, env.SystemMail.Messages())
}
