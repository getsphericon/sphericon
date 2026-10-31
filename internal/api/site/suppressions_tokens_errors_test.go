package site_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/apitoken"
	"github.com/getsphericon/sphericon/ent/suppression"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSiteSuppressionsListPaginatesAndIsScoped(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	res, err := c.SiteSuppressionsList(ctx, siteapi.SiteSuppressionsListParams{
		WorkspaceId: fixtures.AcmePublicID, Page: siteapi.NewOptInt32(1), PageSize: siteapi.NewOptInt32(2),
	})
	require.NoError(t, err)
	page, ok := res.(*siteapi.SiteSuppressionsListOK)
	require.Truef(t, ok, "got %T", res)
	assert.Len(t, page.Items, 2)
	assert.Greater(t, page.TotalItems, int32(2))
	assert.Greater(t, page.Items[0].ID, page.Items[1].ID, "newest first")

	nf, err := c.SiteSuppressionsList(ctx, siteapi.SiteSuppressionsListParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSuppressionsListNotFound{}, nf)

	_, err = env.SiteAnonymous(t).SiteSuppressionsList(ctx, siteapi.SiteSuppressionsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.Error(t, err)
}

func TestSiteSuppressionsErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme, foreign := siteapi.WorkspaceId(fixtures.AcmePublicID), siteapi.WorkspaceId(fixtures.GlobexPublicID)

	r1, err := c.SiteSuppressionsCreate(ctx, &siteapi.SiteCreateSuppressionInput{Destination: "a@b.test"}, siteapi.SiteSuppressionsCreateParams{WorkspaceId: foreign})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSuppressionsCreateNotFound{}, r1)
	r2, err := c.SiteSuppressionsCreate(ctx, &siteapi.SiteCreateSuppressionInput{Destination: "   "}, siteapi.SiteSuppressionsCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSuppressionsCreateUnprocessableEntity{}, r2)

	r3, err := c.SiteSuppressionsDelete(ctx, siteapi.SiteSuppressionsDeleteParams{WorkspaceId: foreign, ID: 100})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSuppressionsDeleteNotFound{}, r3)
	r5, err := c.SiteSuppressionsDelete(ctx, siteapi.SiteSuppressionsDeleteParams{WorkspaceId: acme, ID: idStr(99999)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSuppressionsDeleteNotFound{}, r5)

	// A suppression of another workspace cannot be deleted through Acme.
	r6, err := c.SiteSuppressionsDelete(ctx, siteapi.SiteSuppressionsDeleteParams{WorkspaceId: acme, ID: idStr(fixtures.SuppressionGlobexID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSuppressionsDeleteNotFound{}, r6)

	for _, id := range []int64{fixtures.SuppressionGlobexID, fixtures.SuppressionGhostBounceID} {
		assert.True(t, env.DB.Suppression.Query().Where(suppression.ID(id)).ExistX(ctx), "nothing was deleted")
	}
}

func TestSiteTokensListCreateAndRevoke(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme, foreign := siteapi.WorkspaceId(fixtures.AcmePublicID), siteapi.WorkspaceId(fixtures.GlobexPublicID)

	res, err := c.SiteTokensList(ctx, siteapi.SiteTokensListParams{WorkspaceId: acme})
	require.NoError(t, err)
	list, ok := res.(*siteapi.SiteTokensListOKApplicationJSON)
	require.Truef(t, ok, "got %T", res)
	require.NotEmpty(t, *list)
	assert.Equal(t, siteapi.EntityId(1), (*list)[0].ID)

	nf, err := c.SiteTokensList(ctx, siteapi.SiteTokensListParams{WorkspaceId: foreign})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensListNotFound{}, nf)

	// Create with an expiry, then revoke it.
	created, err := c.SiteTokensCreate(ctx, &siteapi.SiteCreateTokenInput{
		Name: " CI ", Scopes: []siteapi.ApiTokenScope{"contacts:read"},
		ExpiresAt: siteapi.NewOptNilTimestamp(siteapi.Timestamp(time.Now().Add(24 * time.Hour))),
	}, siteapi.SiteTokensCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	tok, ok := created.(*siteapi.SiteCreateTokenResponse)
	require.Truef(t, ok, "got %T", created)
	assert.NotEmpty(t, tok.Token)
	stored, err := env.DB.ApiToken.Get(ctx, int64(tok.Resource.ID))
	require.NoError(t, err)
	assert.Equal(t, "CI", stored.Name)
	assert.NotNil(t, stored.ExpiresAt)

	del, err := c.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{WorkspaceId: acme, ID: tok.Resource.ID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensDeleteNoContent{}, del)
	revoked, err := env.DB.ApiToken.Query().Where(apitoken.ID(stored.ID), apitoken.RevokedAtNotNil()).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, revoked)

	// A revoked token stays in the list, marked as revoked.
	res, err = c.SiteTokensList(ctx, siteapi.SiteTokensListParams{WorkspaceId: acme})
	require.NoError(t, err)
	list, ok = res.(*siteapi.SiteTokensListOKApplicationJSON)
	require.Truef(t, ok, "got %T", res)
	listed, found := lo.Find(*list, func(r siteapi.SiteApiTokenResource) bool { return r.ID == tok.Resource.ID })
	require.True(t, found)
	assert.True(t, listed.RevokedAt.IsSet() && !listed.RevokedAt.Null)

	again, err := c.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{WorkspaceId: acme, ID: tok.Resource.ID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensDeleteNotFound{}, again, "an already revoked token is a 404")
}

func TestSiteTokensErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	ctx := context.Background()
	acme, foreign := siteapi.WorkspaceId(fixtures.AcmePublicID), siteapi.WorkspaceId(fixtures.GlobexPublicID)
	in := &siteapi.SiteCreateTokenInput{Name: "x", Scopes: []siteapi.ApiTokenScope{"contacts:read"}}

	r1, err := owner.SiteTokensCreate(ctx, in, siteapi.SiteTokensCreateParams{WorkspaceId: foreign})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensCreateNotFound{}, r1)
	r2, err := owner.SiteTokensCreate(ctx, &siteapi.SiteCreateTokenInput{Name: " ", Scopes: []siteapi.ApiTokenScope{}}, siteapi.SiteTokensCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensCreateUnprocessableEntity{}, r2)
	r3, err := member.SiteTokensCreate(ctx, in, siteapi.SiteTokensCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensCreateForbidden{}, r3)

	r4, err := owner.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{WorkspaceId: foreign, ID: 1})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensDeleteNotFound{}, r4)
	r5, err := member.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{WorkspaceId: acme, ID: 1})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensDeleteForbidden{}, r5)
	r7, err := owner.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{WorkspaceId: acme, ID: idStr(99999)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensDeleteNotFound{}, r7)

	live, err := env.DB.ApiToken.Query().Where(apitoken.ID(fixtures.AnchorTokenID), apitoken.RevokedAtIsNil()).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, live, "the forbidden member did not revoke the anchor token")
}
