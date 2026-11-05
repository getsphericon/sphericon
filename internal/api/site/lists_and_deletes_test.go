package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Acme's catalogues are listed for its members; a foreign workspace's workspaceID is a
// 404 (never a leak), and anonymous callers are rejected.
func TestSiteCataloguesListForMembersOnly(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme := siteapi.WorkspaceId(fixtures.AcmePublicID)
	foreign := siteapi.WorkspaceId(fixtures.GlobexPublicID)

	automations, err := c.SiteAutomationsList(ctx, siteapi.SiteAutomationsListParams{WorkspaceId: acme})
	require.NoError(t, err)
	assert.NotEmpty(t, automations.Items)
	_, err = c.SiteAutomationsList(ctx, siteapi.SiteAutomationsListParams{WorkspaceId: foreign})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	templates, err := c.SiteTemplatesList(ctx, siteapi.SiteTemplatesListParams{WorkspaceId: acme})
	require.NoError(t, err)
	assert.NotEmpty(t, templates.Items)
	_, err = c.SiteTemplatesList(ctx, siteapi.SiteTemplatesListParams{WorkspaceId: foreign})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	hooks, err := c.SiteWebhooksList(ctx, siteapi.SiteWebhooksListParams{WorkspaceId: acme})
	require.NoError(t, err)
	assert.NotEmpty(t, hooks.Items)
	_, err = c.SiteWebhooksList(ctx, siteapi.SiteWebhooksListParams{WorkspaceId: foreign})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	fields, err := c.SiteCustomFieldsList(ctx, siteapi.SiteCustomFieldsListParams{WorkspaceId: acme})
	require.NoError(t, err)
	assert.NotEmpty(t, fields.Items)
	_, err = c.SiteCustomFieldsList(ctx, siteapi.SiteCustomFieldsListParams{WorkspaceId: foreign})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	anon := env.SiteAnonymous(t)
	_, err = anon.SiteAutomationsList(ctx, siteapi.SiteAutomationsListParams{WorkspaceId: acme})
	require.Error(t, err)
	_, err = anon.SiteTemplatesList(ctx, siteapi.SiteTemplatesListParams{WorkspaceId: acme})
	require.Error(t, err)
	_, err = anon.SiteWebhooksList(ctx, siteapi.SiteWebhooksListParams{WorkspaceId: acme})
	require.Error(t, err)
	_, err = anon.SiteCustomFieldsList(ctx, siteapi.SiteCustomFieldsListParams{WorkspaceId: acme})
	require.Error(t, err)
}

func TestSiteListsHonorPagination(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	page, err := c.SiteWebhooksList(ctx, siteapi.SiteWebhooksListParams{
		WorkspaceId: fixtures.AcmePublicID, Page: siteapi.NewOptInt32(1), PageSize: siteapi.NewOptInt32(1),
	})
	require.NoError(t, err)
	assert.Len(t, page.Items, 1)
	assert.EqualValues(t, 1, page.PageSize)
	assert.Greater(t, page.TotalItems, int32(1))
}

func TestSiteContactsDelete(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	del := func(workspaceID siteapi.WorkspaceId, id siteapi.EntityId) error {
		return c.SiteContactsDelete(ctx, siteapi.SiteContactsDeleteParams{WorkspaceId: workspaceID, ID: id})
	}

	require.NoError(t, del(fixtures.AcmePublicID, idStr(fixtures.ContactBobID)))
	_, err := c.SiteContactsGet(ctx, siteapi.SiteContactsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.ContactBobID)})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	testhelper.SiteRefusal(t, del(fixtures.AcmePublicID, idStr(fixtures.ContactBobID)), http.StatusNotFound, "already gone")
	testhelper.SiteRefusal(t, del(fixtures.GlobexPublicID, idStr(fixtures.ContactAliceID)), http.StatusNotFound, "foreign workspace")
}

func TestSiteInvitationsDelete(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	id := siteapi.EntityId(fixtures.InvitationAcmePendingID)
	params := siteapi.SiteInvitationsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: id}

	// A plain member may not revoke.
	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	err := member.SiteInvitationsDelete(ctx, params)
	testhelper.SiteRefusal(t, err, http.StatusForbidden)

	err = owner.SiteInvitationsDelete(ctx, params)
	require.NoError(t, err)

	// Revoked: deleting again and a foreign workspace are both 404.
	for _, p := range []siteapi.SiteInvitationsDeleteParams{
		params,
		{WorkspaceId: fixtures.GlobexPublicID, ID: id},
	} {
		err = owner.SiteInvitationsDelete(ctx, p)
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
	}
}
