package site_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/getsphericon/sphericon/ent/apitoken"
	"github.com/getsphericon/sphericon/ent/membership"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteTokensCreateListRevoke(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	// Create returns the full secret once.
	created, err := c.SiteTokensCreate(ctx,
		&siteapi.SiteCreateTokenInput{Name: "CI", Scopes: []string{"contacts:read"}},
		siteapi.SiteTokensCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	resp, ok := created.(*siteapi.SiteCreateTokenResponse)
	require.Truef(t, ok, "got %T", created)
	assert.NotEmpty(t, resp.Token)
	assert.NotNil(t, credentials.ParseToken(resp.Token), "returned a well-formed token value")
	assert.Equal(t, "CI", resp.Resource.Name)

	// The new token is persisted and live (selected from the DB by its public prefix).
	row, err := env.DB.ApiToken.Query().Where(apitoken.Prefix(resp.Resource.Prefix)).Only(ctx)
	require.NoError(t, err)
	assert.Nil(t, row.RevokedAt, "freshly created token is not revoked")

	// Revoke the new token; the row is then marked revoked (a soft delete).
	del, err := c.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{
		Slug: fixtures.AcmeSlug,
		ID:   resp.Resource.ID,
	})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensDeleteNoContent{}, del)

	revoked, err := env.DB.ApiToken.Query().Where(apitoken.Prefix(resp.Resource.Prefix)).Only(ctx)
	require.NoError(t, err)
	assert.NotNil(t, revoked.RevokedAt, "revoked token carries a revoked_at timestamp")
}

func TestSiteTokensDeleteUnknown(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	del, err := c.SiteTokensDelete(context.Background(), siteapi.SiteTokensDeleteParams{
		Slug: fixtures.AcmeSlug,
		ID:   "999999",
	})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensDeleteNotFound{}, del)
}

func TestSiteTokensWorkspaceNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteTokensList(context.Background(), siteapi.SiteTokensListParams{Slug: "does-not-exist"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.ProblemDetails{}, res)
}

func TestSiteTokensNeedAnOwnerOrAdmin(t *testing.T) {
	env := testhelper.Setup(t)
	addMember(t, env, "admin@acme.test", membership.RoleAdmin)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)
	ctx := context.Background()

	created, err := c.SiteTokensCreate(ctx,
		&siteapi.SiteCreateTokenInput{Name: "sneaky", Scopes: []string{"contacts:write"}},
		siteapi.SiteTokensCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensCreateForbidden{}, created)

	tok := env.DB.ApiToken.Query().FirstX(ctx)
	deleted, err := c.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{Slug: fixtures.AcmeSlug, ID: siteapi.EntityId(strconv.FormatInt(tok.ID, 10))})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensDeleteForbidden{}, deleted)

	ok, err := env.SiteActor(t, "admin@acme.test").SiteTokensCreate(ctx,
		&siteapi.SiteCreateTokenInput{Name: "ci", Scopes: []string{"contacts:read"}},
		siteapi.SiteTokensCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteCreateTokenResponse{}, ok)
}

func TestSiteTokensCreateRefusesAnUnknownScope(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	res, err := c.SiteTokensCreate(context.Background(),
		&siteapi.SiteCreateTokenInput{Name: "x", Scopes: []string{"contacts:read", "root:all"}},
		siteapi.SiteTokensCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensCreateUnprocessableEntity{}, res)
}
