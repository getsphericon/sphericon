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
		Slug: fixtures.AcmeSlug, Page: siteapi.NewOptInt32(1), PageSize: siteapi.NewOptInt32(2),
	})
	require.NoError(t, err)
	page, ok := res.(*siteapi.SiteSuppressionsListOK)
	require.Truef(t, ok, "got %T", res)
	assert.Len(t, page.Items, 2)
	assert.Greater(t, page.TotalItems, int32(2))
	assert.Greater(t, page.Items[0].ID, page.Items[1].ID, "newest first")

	nf, err := c.SiteSuppressionsList(ctx, siteapi.SiteSuppressionsListParams{Slug: fixtures.GlobexSlug})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSuppressionsListNotFound{}, nf)

	_, err = env.SiteAnonymous(t).SiteSuppressionsList(ctx, siteapi.SiteSuppressionsListParams{Slug: fixtures.AcmeSlug})
	require.Error(t, err)
}

func TestSiteSuppressionsErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme, foreign := fixtures.AcmeSlug, fixtures.GlobexSlug

	r1, err := c.SiteSuppressionsCreate(ctx, &siteapi.SiteCreateSuppressionInput{Destination: "a@b.test"}, siteapi.SiteSuppressionsCreateParams{Slug: foreign})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSuppressionsCreateNotFound{}, r1)
	r2, err := c.SiteSuppressionsCreate(ctx, &siteapi.SiteCreateSuppressionInput{Destination: "   "}, siteapi.SiteSuppressionsCreateParams{Slug: acme})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSuppressionsCreateUnprocessableEntity{}, r2)

	before, err := env.DB.Suppression.Query().Count(ctx)
	require.NoError(t, err)

	r3, err := c.SiteSuppressionsDelete(ctx, siteapi.SiteSuppressionsDeleteParams{Slug: foreign, ID: 100})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSuppressionsDeleteNotFound{}, r3)
	r5, err := c.SiteSuppressionsDelete(ctx, siteapi.SiteSuppressionsDeleteParams{Slug: acme, ID: idStr(99999)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSuppressionsDeleteNotFound{}, r5)

	// A suppression of another workspace cannot be deleted through Acme.
	other, err := env.DB.Suppression.Create().SetWorkspaceID(fixtures.GlobexID).
		SetChannel(suppression.ChannelEmail).SetDestination("x@globex.test").SetReason(suppression.ReasonManual).Save(ctx)
	require.NoError(t, err)
	r6, err := c.SiteSuppressionsDelete(ctx, siteapi.SiteSuppressionsDeleteParams{Slug: acme, ID: idStr(int64(other.ID))})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSuppressionsDeleteNotFound{}, r6)

	after, err := env.DB.Suppression.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, before+1, after, "nothing was deleted")
}

func TestSiteTokensListCreateAndRevoke(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme, foreign := fixtures.AcmeSlug, fixtures.GlobexSlug

	res, err := c.SiteTokensList(ctx, siteapi.SiteTokensListParams{Slug: acme})
	require.NoError(t, err)
	list, ok := res.(*siteapi.SiteTokensListOKApplicationJSON)
	require.Truef(t, ok, "got %T", res)
	require.NotEmpty(t, *list)
	assert.Equal(t, siteapi.EntityId(1), (*list)[0].ID)

	nf, err := c.SiteTokensList(ctx, siteapi.SiteTokensListParams{Slug: foreign})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.ProblemDetails{}, nf)

	// Create with an expiry, then revoke it.
	created, err := c.SiteTokensCreate(ctx, &siteapi.SiteCreateTokenInput{
		Name: " CI ", Scopes: []string{"contacts:read"},
		ExpiresAt: siteapi.NewOptNilTimestamp(siteapi.Timestamp(time.Now().Add(24 * time.Hour))),
	}, siteapi.SiteTokensCreateParams{Slug: acme})
	require.NoError(t, err)
	tok, ok := created.(*siteapi.SiteCreateTokenResponse)
	require.Truef(t, ok, "got %T", created)
	assert.NotEmpty(t, tok.Token)
	stored, err := env.DB.ApiToken.Get(ctx, int64(tok.Resource.ID))
	require.NoError(t, err)
	assert.Equal(t, "CI", stored.Name)
	assert.NotNil(t, stored.ExpiresAt)

	del, err := c.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{Slug: acme, ID: tok.Resource.ID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensDeleteNoContent{}, del)
	revoked, err := env.DB.ApiToken.Query().Where(apitoken.ID(stored.ID), apitoken.RevokedAtNotNil()).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, revoked)

	// A revoked token stays in the list, marked as revoked.
	res, err = c.SiteTokensList(ctx, siteapi.SiteTokensListParams{Slug: acme})
	require.NoError(t, err)
	list, ok = res.(*siteapi.SiteTokensListOKApplicationJSON)
	require.Truef(t, ok, "got %T", res)
	listed, found := lo.Find(*list, func(r siteapi.SiteApiTokenResource) bool { return r.ID == tok.Resource.ID })
	require.True(t, found)
	assert.True(t, listed.RevokedAt.IsSet() && !listed.RevokedAt.Null)

	again, err := c.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{Slug: acme, ID: tok.Resource.ID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensDeleteNotFound{}, again, "an already revoked token is a 404")
}

func TestSiteTokensErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	ctx := context.Background()
	acme, foreign := fixtures.AcmeSlug, fixtures.GlobexSlug
	in := &siteapi.SiteCreateTokenInput{Name: "x", Scopes: []string{"contacts:read"}}

	r1, err := owner.SiteTokensCreate(ctx, in, siteapi.SiteTokensCreateParams{Slug: foreign})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensCreateNotFound{}, r1)
	r2, err := owner.SiteTokensCreate(ctx, &siteapi.SiteCreateTokenInput{Name: " ", Scopes: []string{}}, siteapi.SiteTokensCreateParams{Slug: acme})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensCreateUnprocessableEntity{}, r2)
	r3, err := member.SiteTokensCreate(ctx, in, siteapi.SiteTokensCreateParams{Slug: acme})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensCreateForbidden{}, r3)

	r4, err := owner.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{Slug: foreign, ID: 1})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensDeleteNotFound{}, r4)
	r5, err := member.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{Slug: acme, ID: 1})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensDeleteForbidden{}, r5)
	r7, err := owner.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{Slug: acme, ID: idStr(99999)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTokensDeleteNotFound{}, r7)

	live, err := env.DB.ApiToken.Query().Where(apitoken.ID(fixtures.AnchorTokenID), apitoken.RevokedAtIsNil()).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, live, "the forbidden member did not revoke the anchor token")
}
