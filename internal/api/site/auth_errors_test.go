package site_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
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

// Verifying an address creates a Workspace named "Personal" for each User: the name is not
// unique, so two Users both get "Personal", each under an id of its own.
func TestSiteAuthVerifyNamesEveryFirstWorkspacePersonal(t *testing.T) {
	env := testhelper.Setup(t)

	wsOf := func(email string) (*ent.Workspace, membership.Role) {
		registerAndVerify(t, env, "Someone", email, "password123")
		u, err := env.DB.User.Query().Where(entuser.Email(email)).Only(t.Context())
		require.NoError(t, err)
		ws, err := env.DB.Workspace.Query().Where(workspace.HasMembershipsWith(membership.UserID(u.ID))).Only(t.Context())
		require.NoError(t, err)
		m, err := env.DB.Membership.Query().Where(membership.UserID(u.ID), membership.WorkspaceID(ws.ID)).Only(t.Context())
		require.NoError(t, err)
		return ws, m.Role
	}

	first, role := wsOf("one@example.com")
	assert.Equal(t, "Personal", first.Name)
	assert.Equal(t, membership.RoleOwner, role)

	second, _ := wsOf("two@example.com")
	assert.Equal(t, "Personal", second.Name)
	assert.NotEqual(t, first.PublicID, second.PublicID)
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
		res, err := c.SiteAuthVerifyEmail(ctx, &siteapi.SiteVerifyEmailInput{Token: "garbage", Password: "password123"})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.ProblemDetails{}, res)
	})

	t.Run("verify rejects a token for a user that no longer exists", func(t *testing.T) {
		tok, err := s.Mint(authtoken.PurposeEmailVerify, 987654, "", time.Hour, map[string]string{"email": "ghost@example.com"})
		require.NoError(t, err)
		res, err := c.SiteAuthVerifyEmail(ctx, &siteapi.SiteVerifyEmailInput{Token: tok, Password: "password123"})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.ProblemDetails{}, res)
	})

	t.Run("confirm email change rejects garbage", func(t *testing.T) {
		res, err := c.SiteAuthConfirmEmailChange(ctx, &siteapi.SiteConfirmEmailChangeInput{Token: "garbage"}, siteapi.SiteAuthConfirmEmailChangeParams{})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.ProblemDetails{}, res)
	})

	t.Run("confirm email change rejects a token without a new address", func(t *testing.T) {
		tok, err := s.Mint(authtoken.PurposeEmailChange, john.ID, john.Email, time.Hour, nil)
		require.NoError(t, err)
		res, err := c.SiteAuthConfirmEmailChange(ctx, &siteapi.SiteConfirmEmailChangeInput{Token: tok}, siteapi.SiteAuthConfirmEmailChangeParams{})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.ProblemDetails{}, res)
	})

	t.Run("confirm email change never says the address is in use", func(t *testing.T) {
		tok, err := s.Mint(authtoken.PurposeEmailChange, john.ID, john.Email, time.Hour, map[string]string{"new": fixtures.MemberMaryEmail})
		require.NoError(t, err)
		res, err := c.SiteAuthConfirmEmailChange(ctx, &siteapi.SiteConfirmEmailChangeInput{Token: tok}, siteapi.SiteAuthConfirmEmailChangeParams{})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.ProblemDetails{}, res, "to the confirmer a Taken address is an invalid link (ADR 0031)")
		after, err := env.DB.User.Get(ctx, john.ID)
		require.NoError(t, err)
		assert.Equal(t, fixtures.OwnerJohnEmail, after.Email)
	})
}

func TestSiteUserEmailChangeEdgeCases(t *testing.T) {
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
}
