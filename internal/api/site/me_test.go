package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// loginStatus drives the real login endpoint and returns the status code,
// used to prove a password change took effect end to end.
func loginStatus(t *testing.T, env *testhelper.TestEnv, email, password string) int {
	t.Helper()
	return postLogin(t, env, email, password).Code
}

func TestSiteUserGetMe(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	me, err := c.SiteUserGetMe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "John", me.Name)
	assert.Equal(t, siteapi.EmailAddress(fixtures.OwnerJohnEmail), me.Email)
}

func TestSiteUserUpdateMeName(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	res, err := c.SiteUserUpdateMe(ctx, &siteapi.SiteUpdateMeInput{Name: siteapi.NewOptString("Renamed")})
	require.NoError(t, err)
	updated, ok := res.(*siteapi.SiteUserResourceHeaders)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, "Renamed", updated.Response.Name)

	// The change persists.
	me, err := c.SiteUserGetMe(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", me.Name)
}

func TestSiteUserUpdateMeRejectsBlankName(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteUserUpdateMe(context.Background(),
		&siteapi.SiteUpdateMeInput{Name: siteapi.NewOptString("   ")})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteUserUpdateMeUnprocessableEntity{}, res)
}

// The User time zone (ADR 0068) is set and cleared through the profile, and a change of the name
// alone keeps it.
func TestSiteUserUpdateMeTimezone(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	me, err := c.SiteUserGetMe(ctx)
	require.NoError(t, err)
	assert.True(t, me.Timezone.IsNull(), "no zone until the User chooses one")

	_, err = c.SiteUserUpdateMe(ctx, &siteapi.SiteUpdateMeInput{Timezone: siteapi.NewOptNilTimeZoneName("Europe/Berlin")})
	require.NoError(t, err)
	_, err = c.SiteUserUpdateMe(ctx, &siteapi.SiteUpdateMeInput{Name: siteapi.NewOptString("Renamed")})
	require.NoError(t, err)
	me, err = c.SiteUserGetMe(ctx)
	require.NoError(t, err)
	zone, ok := me.Timezone.Get()
	require.True(t, ok)
	assert.Equal(t, siteapi.TimeZoneName("Europe/Berlin"), zone)

	_, err = c.SiteUserUpdateMe(ctx, &siteapi.SiteUpdateMeInput{Timezone: siteapi.OptNilTimeZoneName{Set: true, Null: true}})
	require.NoError(t, err)
	me, err = c.SiteUserGetMe(ctx)
	require.NoError(t, err)
	assert.True(t, me.Timezone.IsNull(), "null clears the zone")
}

func TestSiteUserUpdateMeRejectsUnknownTimezone(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteUserUpdateMe(context.Background(),
		&siteapi.SiteUpdateMeInput{Timezone: siteapi.NewOptNilTimeZoneName("Mars/Olympus")})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteUserUpdateMeUnprocessableEntity{}, res)
}

func TestSiteUserUpdateMePasswordWrongCurrent(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteUserUpdateMe(context.Background(), &siteapi.SiteUpdateMeInput{
		CurrentPassword: siteapi.NewOptString("wrong"),
		NewPassword:     siteapi.NewOptString("newsecret123"),
	})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteUserUpdateMeForbidden{}, res)

	// Original password still works.
	assert.Equal(t, http.StatusOK, loginStatus(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword))
}

func TestSiteUserUpdateMePasswordChange(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteUserUpdateMe(context.Background(), &siteapi.SiteUpdateMeInput{
		CurrentPassword: siteapi.NewOptString(fixtures.OwnerJohnPassword),
		NewPassword:     siteapi.NewOptString("newsecret123"),
	})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteUserResourceHeaders{}, res)

	// The new password works; the old one no longer does.
	assert.Equal(t, http.StatusOK, loginStatus(t, env, fixtures.OwnerJohnEmail, "newsecret123"))
	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword))
}

func TestSiteUserUpdateMeWrongCurrentPasswordsAreThrottled(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{LoginFailures: 3}))
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	for range 3 {
		res, err := c.SiteUserUpdateMe(t.Context(), &siteapi.SiteUpdateMeInput{
			CurrentPassword: siteapi.NewOptString("wrong"),
			NewPassword:     siteapi.NewOptString("newsecret123"),
		})
		require.NoError(t, err)
		require.IsType(t, &siteapi.SiteUserUpdateMeForbidden{}, res)
	}

	res, err := c.SiteUserUpdateMe(t.Context(), &siteapi.SiteUpdateMeInput{
		CurrentPassword: siteapi.NewOptString(fixtures.OwnerJohnPassword),
		NewPassword:     siteapi.NewOptString("newsecret123"),
	})

	require.NoError(t, err)
	assert.IsType(t, &siteapi.ProblemDetailsHeaders{}, res, "while the delay runs even the right password is refused")
}

func TestSiteUserEmailChangeWrongCurrentPasswordsAreThrottled(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{LoginFailures: 3}))
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	for range 3 {
		res, err := c.SiteUserEmailChange(t.Context(), &siteapi.SiteEmailChangeInput{
			NewEmail: "new-john@gmail.com", CurrentPassword: "wrong",
		})
		require.NoError(t, err)
		require.IsType(t, &siteapi.SiteUserEmailChangeForbidden{}, res)
	}

	res, err := c.SiteUserEmailChange(t.Context(), &siteapi.SiteEmailChangeInput{
		NewEmail: "new-john@gmail.com", CurrentPassword: fixtures.OwnerJohnPassword,
	})

	require.NoError(t, err)
	assert.IsType(t, &siteapi.ProblemDetailsHeaders{}, res)
}

func TestAWrongCurrentPasswordOnTheProfileCountsAgainstTheLoginBudget(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{LoginFailures: 3}))
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	for range 3 {
		_, err := c.SiteUserUpdateMe(t.Context(), &siteapi.SiteUpdateMeInput{
			CurrentPassword: siteapi.NewOptString("wrong"),
			NewPassword:     siteapi.NewOptString("newsecret123"),
		})
		require.NoError(t, err)
	}

	assert.Equal(t, http.StatusTooManyRequests, loginStatus(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword))
}
