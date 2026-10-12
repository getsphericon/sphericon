package site_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Acme's catalogues are listed for its members; a foreign workspace's slug is a
// 404 (never a leak), and anonymous callers are rejected.
func TestSiteCataloguesListForMembersOnly(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme := fixtures.AcmeSlug
	foreign := fixtures.GlobexSlug

	automations, err := c.SiteAutomationsList(ctx, siteapi.SiteAutomationsListParams{Slug: acme})
	require.NoError(t, err)
	assert.NotEmpty(t, automations.(*siteapi.SiteAutomationsListOK).Items)
	res, err := c.SiteAutomationsList(ctx, siteapi.SiteAutomationsListParams{Slug: foreign})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAutomationsListNotFound{}, res)

	templates, err := c.SiteTemplatesList(ctx, siteapi.SiteTemplatesListParams{Slug: acme})
	require.NoError(t, err)
	assert.NotEmpty(t, templates.(*siteapi.SiteTemplatesListOK).Items)
	tres, err := c.SiteTemplatesList(ctx, siteapi.SiteTemplatesListParams{Slug: foreign})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTemplatesListNotFound{}, tres)

	hooks, err := c.SiteWebhooksList(ctx, siteapi.SiteWebhooksListParams{Slug: acme})
	require.NoError(t, err)
	assert.NotEmpty(t, hooks.(*siteapi.SiteWebhooksListOK).Items)
	hres, err := c.SiteWebhooksList(ctx, siteapi.SiteWebhooksListParams{Slug: foreign})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWebhooksListNotFound{}, hres)

	fields, err := c.SiteCustomFieldsList(ctx, siteapi.SiteCustomFieldsListParams{Slug: acme})
	require.NoError(t, err)
	assert.NotEmpty(t, fields.(*siteapi.SiteCustomFieldsListOK).Items)
	fres, err := c.SiteCustomFieldsList(ctx, siteapi.SiteCustomFieldsListParams{Slug: foreign})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteCustomFieldsListNotFound{}, fres)

	anon := env.SiteAnonymous(t)
	_, err = anon.SiteAutomationsList(ctx, siteapi.SiteAutomationsListParams{Slug: acme})
	require.Error(t, err)
	_, err = anon.SiteTemplatesList(ctx, siteapi.SiteTemplatesListParams{Slug: acme})
	require.Error(t, err)
	_, err = anon.SiteWebhooksList(ctx, siteapi.SiteWebhooksListParams{Slug: acme})
	require.Error(t, err)
	_, err = anon.SiteCustomFieldsList(ctx, siteapi.SiteCustomFieldsListParams{Slug: acme})
	require.Error(t, err)
}

func TestSiteListsHonorPagination(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	res, err := c.SiteWebhooksList(ctx, siteapi.SiteWebhooksListParams{
		Slug: fixtures.AcmeSlug, Page: siteapi.NewOptInt32(1), PageSize: siteapi.NewOptInt32(1),
	})
	require.NoError(t, err)
	page := res.(*siteapi.SiteWebhooksListOK)
	assert.Len(t, page.Items, 1)
	assert.EqualValues(t, 1, page.PageSize)
	assert.Greater(t, page.TotalItems, int32(1))
}

func TestSiteContactsDelete(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	del := func(slug string, id siteapi.EntityId) siteapi.SiteContactsDeleteRes {
		res, err := c.SiteContactsDelete(ctx, siteapi.SiteContactsDeleteParams{Slug: slug, ID: id})
		require.NoError(t, err)
		return res
	}

	assert.IsType(t, &siteapi.SiteContactsDeleteNoContent{}, del(fixtures.AcmeSlug, idStr(fixtures.ContactBobID)))
	get, err := c.SiteContactsGet(ctx, siteapi.SiteContactsGetParams{Slug: fixtures.AcmeSlug, ID: idStr(fixtures.ContactBobID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteContactsGetNotFound{}, get)

	assert.IsType(t, &siteapi.SiteContactsDeleteNotFound{}, del(fixtures.AcmeSlug, idStr(fixtures.ContactBobID)), "already gone")
	assert.IsType(t, &siteapi.SiteContactsDeleteNotFound{}, del(fixtures.GlobexSlug, idStr(fixtures.ContactAliceID)), "foreign workspace")
	assert.IsType(t, &siteapi.SiteContactsDeleteBadRequest{}, del(fixtures.AcmeSlug, "not-a-number"))
}

// Two contacts cannot share an alias key: the second create is a 409 naming the field.
func TestSiteContactsCreateDuplicateEmailConflicts(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteContactsCreate(context.Background(),
		&siteapi.SiteCreateContactInput{Email: siteapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail)},
		siteapi.SiteContactsCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	conflict, ok := res.(*siteapi.SiteContactsCreateConflict)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, conflict.Errors.Value, "email")
}

func TestSiteInvitationsDelete(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	created, err := owner.SiteInvitationsCreate(ctx,
		&siteapi.SiteCreateInvitationInput{Email: "revoke-me@acme.test", Role: siteapi.SiteInvitableRoleMember},
		siteapi.SiteInvitationsCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	id := created.(*siteapi.SiteCreateInvitationResponse).Resource.ID
	params := siteapi.SiteInvitationsDeleteParams{Slug: fixtures.AcmeSlug, ID: id}

	// A plain member may not revoke.
	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	res, err := member.SiteInvitationsDelete(ctx, params)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteInvitationsDeleteForbidden{}, res)

	res, err = owner.SiteInvitationsDelete(ctx, params)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteInvitationsDeleteNoContent{}, res)

	// Revoked: deleting again, an id that overflows, and a foreign workspace are all 404.
	for _, p := range []siteapi.SiteInvitationsDeleteParams{
		params,
		{Slug: fixtures.AcmeSlug, ID: "99999999999999999999"}, // numeric, but overflows int64
		{Slug: fixtures.GlobexSlug, ID: id},
	} {
		res, err = owner.SiteInvitationsDelete(ctx, p)
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteInvitationsDeleteNotFound{}, res)
	}
}
