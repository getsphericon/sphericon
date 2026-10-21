package site_test

import (
	"context"
	"testing"

	"github.com/getsphericon/sphericon/ent/membership"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An endpoint carries its signing secret and decides where events are delivered: every
// operation on one, reads included, needs an Owner or an Admin.
func TestSiteWebhooksNeedAnOwnerOrAdmin(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	created, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteWebhooksCreate(ctx,
		&siteapi.SiteCreateWebhookEndpointInput{URL: "https://example.com/hook"},
		siteapi.SiteWebhooksCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	id := created.(*siteapi.SiteWebhookEndpointResource).ID

	member := env.SiteActor(t, fixtures.MemberMaryEmail)

	list, err := member.SiteWebhooksList(ctx, siteapi.SiteWebhooksListParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteWebhooksListForbidden), list)

	got, err := member.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteWebhooksGetForbidden), got)

	made, err := member.SiteWebhooksCreate(ctx, &siteapi.SiteCreateWebhookEndpointInput{URL: "https://attacker.example/hook"},
		siteapi.SiteWebhooksCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteWebhooksCreateForbidden), made)

	updated, err := member.SiteWebhooksUpdate(ctx, &siteapi.SiteUpdateWebhookEndpointInput{URL: siteapi.NewOptString("https://attacker.example/hook")},
		siteapi.SiteWebhooksUpdateParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteWebhooksUpdateForbidden), updated)

	regenerated, err := member.SiteWebhooksRegenerateSecret(ctx, siteapi.SiteWebhooksRegenerateSecretParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteWebhooksRegenerateSecretForbidden), regenerated)

	tested, err := member.SiteWebhooksTest(ctx, &siteapi.SiteTestWebhookInput{EventType: "contact.created"},
		siteapi.SiteWebhooksTestParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteWebhooksTestForbidden), tested)

	deleted, err := member.SiteWebhooksDelete(ctx, siteapi.SiteWebhooksDeleteParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteWebhooksDeleteForbidden), deleted)

	// The refused calls changed nothing: the owner still sees the one endpoint, unchanged.
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	read, err := owner.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/hook", read.(*siteapi.SiteWebhookEndpointResource).URL)

	// An admin manages them.
	addMember(t, env, "admin@acme.test", membership.RoleAdmin)
	adminList, err := env.SiteActor(t, "admin@acme.test").SiteWebhooksList(ctx, siteapi.SiteWebhooksListParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteWebhooksListOK), adminList)
}
