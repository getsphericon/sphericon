package site_test

import (
	"context"
	"net/http"
	"testing"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
