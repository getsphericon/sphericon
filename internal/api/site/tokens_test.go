package site_test

import (
	"context"
	"testing"

	"github.com/getsphericon/sphericon/ent/apitoken"
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
		&siteapi.SiteCreateTokenInput{Name: "CI", Scopes: []siteapi.ApiTokenScope{"contacts:read"}},
		siteapi.SiteTokensCreateParams{WorkspaceId: fixtures.AcmePublicID})
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
		WorkspaceId: fixtures.AcmePublicID,
		ID:          resp.Resource.ID,
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
		WorkspaceId: fixtures.AcmePublicID,
		ID:          999999,
	})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensDeleteNotFound{}, del)
}

func TestSiteTokensWorkspaceNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteTokensList(context.Background(), siteapi.SiteTokensListParams{WorkspaceId: "NoSuchWs"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.ProblemDetails{}, res)
}

func TestSiteTokensNeedAnOwnerOrAdmin(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)
	ctx := context.Background()

	created, err := c.SiteTokensCreate(ctx,
		&siteapi.SiteCreateTokenInput{Name: "sneaky", Scopes: []siteapi.ApiTokenScope{"contacts:write"}},
		siteapi.SiteTokensCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensCreateForbidden{}, created)

	tok := env.DB.ApiToken.Query().FirstX(ctx)
	deleted, err := c.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(tok.ID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensDeleteForbidden{}, deleted)

	ok, err := env.SiteActor(t, fixtures.AdminAliceEmail).SiteTokensCreate(ctx,
		&siteapi.SiteCreateTokenInput{Name: "ci", Scopes: []siteapi.ApiTokenScope{"contacts:read"}},
		siteapi.SiteTokensCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteCreateTokenResponse{}, ok)
}
