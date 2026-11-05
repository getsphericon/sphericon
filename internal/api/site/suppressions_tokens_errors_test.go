package site_test

import (
	"context"
	"net/http"
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

	page, err := c.SiteSuppressionsList(ctx, siteapi.SiteSuppressionsListParams{
		WorkspaceId: fixtures.AcmePublicID, Page: siteapi.NewOptInt32(1), PageSize: siteapi.NewOptInt32(2),
	})
	require.NoError(t, err)
	assert.Len(t, page.Items, 2)
	assert.Greater(t, page.TotalItems, int32(2))
	assert.Greater(t, page.Items[0].ID, page.Items[1].ID, "newest first")

	_, err = c.SiteSuppressionsList(ctx, siteapi.SiteSuppressionsListParams{WorkspaceId: fixtures.GlobexPublicID})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	_, err = env.SiteAnonymous(t).SiteSuppressionsList(ctx, siteapi.SiteSuppressionsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.Error(t, err)
}

func TestSiteSuppressionsErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme, foreign := siteapi.WorkspaceId(fixtures.AcmePublicID), siteapi.WorkspaceId(fixtures.GlobexPublicID)

	_, err := c.SiteSuppressionsCreate(ctx, &siteapi.SiteCreateSuppressionInput{Destination: "a@b.test"}, siteapi.SiteSuppressionsCreateParams{WorkspaceId: foreign})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteSuppressionsCreate(ctx, &siteapi.SiteCreateSuppressionInput{Destination: "   "}, siteapi.SiteSuppressionsCreateParams{WorkspaceId: acme})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

	err = c.SiteSuppressionsDelete(ctx, siteapi.SiteSuppressionsDeleteParams{WorkspaceId: foreign, ID: 100})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	err = c.SiteSuppressionsDelete(ctx, siteapi.SiteSuppressionsDeleteParams{WorkspaceId: acme, ID: idStr(99999)})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	// A suppression of another workspace cannot be deleted through Acme.
	err = c.SiteSuppressionsDelete(ctx, siteapi.SiteSuppressionsDeleteParams{WorkspaceId: acme, ID: idStr(fixtures.SuppressionGlobexID)})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	for _, id := range []int64{fixtures.SuppressionGlobexID, fixtures.SuppressionGhostBounceID} {
		assert.True(t, env.DB.Suppression.Query().Where(suppression.ID(id)).ExistX(ctx), "nothing was deleted")
	}
}

func TestSiteTokensListCreateAndRevoke(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme, foreign := siteapi.WorkspaceId(fixtures.AcmePublicID), siteapi.WorkspaceId(fixtures.GlobexPublicID)

	list, err := c.SiteTokensList(ctx, siteapi.SiteTokensListParams{WorkspaceId: acme})
	require.NoError(t, err)
	require.NotEmpty(t, list)
	assert.Equal(t, siteapi.EntityId(1), list[0].ID)

	_, err = c.SiteTokensList(ctx, siteapi.SiteTokensListParams{WorkspaceId: foreign})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	// Create with an expiry, then revoke it.
	tok, err := c.SiteTokensCreate(ctx, &siteapi.SiteCreateTokenInput{
		Name: " CI ", Scopes: []siteapi.ApiTokenScope{"contacts:read"},
		ExpiresAt: siteapi.NewOptNilTimestamp(siteapi.Timestamp(time.Now().Add(24 * time.Hour))),
	}, siteapi.SiteTokensCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	assert.NotEmpty(t, tok.Token)
	stored, err := env.DB.ApiToken.Get(ctx, int64(tok.Resource.ID))
	require.NoError(t, err)
	assert.Equal(t, "CI", stored.Name)
	assert.NotNil(t, stored.ExpiresAt)

	err = c.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{WorkspaceId: acme, ID: tok.Resource.ID})
	require.NoError(t, err)
	revoked, err := env.DB.ApiToken.Query().Where(apitoken.ID(stored.ID), apitoken.RevokedAtNotNil()).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, revoked)

	// A revoked token stays in the list, marked as revoked.
	list, err = c.SiteTokensList(ctx, siteapi.SiteTokensListParams{WorkspaceId: acme})
	require.NoError(t, err)
	listed, found := lo.Find(list, func(r siteapi.SiteApiTokenResource) bool { return r.ID == tok.Resource.ID })
	require.True(t, found)
	assert.True(t, listed.RevokedAt.IsSet() && !listed.RevokedAt.Null)

	err = c.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{WorkspaceId: acme, ID: tok.Resource.ID})
	testhelper.SiteRefusal(t, err, http.StatusNotFound, "an already revoked token is a 404")
}

func TestSiteTokensErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	ctx := context.Background()
	acme, foreign := siteapi.WorkspaceId(fixtures.AcmePublicID), siteapi.WorkspaceId(fixtures.GlobexPublicID)
	in := &siteapi.SiteCreateTokenInput{Name: "x", Scopes: []siteapi.ApiTokenScope{"contacts:read"}}

	_, err := owner.SiteTokensCreate(ctx, in, siteapi.SiteTokensCreateParams{WorkspaceId: foreign})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = owner.SiteTokensCreate(ctx, &siteapi.SiteCreateTokenInput{Name: " ", Scopes: []siteapi.ApiTokenScope{}}, siteapi.SiteTokensCreateParams{WorkspaceId: acme})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	_, err = member.SiteTokensCreate(ctx, in, siteapi.SiteTokensCreateParams{WorkspaceId: acme})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)

	err = owner.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{WorkspaceId: foreign, ID: 1})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	err = member.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{WorkspaceId: acme, ID: 1})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)
	err = owner.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{WorkspaceId: acme, ID: idStr(99999)})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	live, err := env.DB.ApiToken.Query().Where(apitoken.ID(fixtures.AnchorTokenID), apitoken.RevokedAtIsNil()).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, live, "the forbidden member did not revoke the anchor token")
}
