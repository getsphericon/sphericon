package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/samber/lo"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/apitoken"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
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
	resp := created
	assert.NotEmpty(t, resp.Token)
	assert.NotNil(t, credentials.ParseToken(resp.Token), "returned a well-formed token value")
	assert.Equal(t, "CI", resp.Resource.Name)

	// The new token is persisted and live (selected from the DB by its public prefix).
	row, err := env.DB.ApiToken.Query().Where(apitoken.Prefix(resp.Resource.Prefix)).Only(ctx)
	require.NoError(t, err)
	assert.Nil(t, row.RevokedAt, "freshly created token is not revoked")

	// Revoke the new token; the row is then marked revoked (a soft delete).
	err = c.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{
		WorkspaceId: fixtures.AcmePublicID,
		ID:          resp.Resource.ID,
	})
	require.NoError(t, err)

	revoked, err := env.DB.ApiToken.Query().Where(apitoken.Prefix(resp.Resource.Prefix)).Only(ctx)
	require.NoError(t, err)
	assert.NotNil(t, revoked.RevokedAt, "revoked token carries a revoked_at timestamp")
}

func TestSiteTokensListIsInCreationOrderAndKeepsRevoked(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	list, err := c.SiteTokensList(ctx, siteapi.SiteTokensListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	got := lo.Map(list, func(it siteapi.SiteApiTokenResource, _ int) int64 { return int64(it.ID) })

	want, err := env.DB.ApiToken.Query().
		Where(apitoken.WorkspaceID(fixtures.AcmeID)).
		Order(ent.Asc(apitoken.FieldID)).
		IDs(ctx)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestSiteTokensWorkspaceNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := c.SiteTokensList(context.Background(), siteapi.SiteTokensListParams{WorkspaceId: "NoSuchWs"})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}

func TestSiteTokensNeedAnOwnerOrAdmin(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)
	ctx := context.Background()

	_, err := c.SiteTokensCreate(ctx,
		&siteapi.SiteCreateTokenInput{Name: "sneaky", Scopes: []siteapi.ApiTokenScope{"contacts:write"}},
		siteapi.SiteTokensCreateParams{WorkspaceId: fixtures.AcmePublicID})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)

	tok := env.DB.ApiToken.Query().FirstX(ctx)
	err = c.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(tok.ID)})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)

	_, err = env.SiteActor(t, fixtures.AdminAliceEmail).SiteTokensCreate(ctx,
		&siteapi.SiteCreateTokenInput{Name: "ci", Scopes: []siteapi.ApiTokenScope{"contacts:read"}},
		siteapi.SiteTokensCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
}

func TestSiteTokensRename(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	params := siteapi.SiteTokensUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.ProductionTokenID}

	renamed, err := c.SiteTokensUpdate(ctx, &siteapi.SiteUpdateTokenInput{Name: "  Deploys "}, params)
	require.NoError(t, err)
	res := renamed
	assert.Equal(t, "Deploys", res.Name)

	_, err = c.SiteTokensUpdate(ctx, &siteapi.SiteUpdateTokenInput{Name: " "}, params)
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

	_, err = c.SiteTokensUpdate(ctx, &siteapi.SiteUpdateTokenInput{Name: "late"},
		siteapi.SiteTokensUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.RevokedTokenID})
	testhelper.SiteRefusal(t, err, http.StatusNotFound, "a revoked token cannot be renamed")

	_, err = env.SiteActor(t, fixtures.MemberMaryEmail).SiteTokensUpdate(ctx, &siteapi.SiteUpdateTokenInput{Name: "x"}, params)
	testhelper.SiteRefusal(t, err, http.StatusForbidden)
}
