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

// An endpoint carries its signing secret and decides where events are delivered: every
// operation on one, reads included, needs an Owner or an Admin.
func TestSiteWebhooksNeedAnOwnerOrAdmin(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	created, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteWebhooksCreate(ctx,
		&siteapi.CreateWebhookInput{URL: "https://example.com/hook"},
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

	made, err := member.SiteWebhooksCreate(ctx, &siteapi.CreateWebhookInput{URL: "https://attacker.example/hook"},
		siteapi.SiteWebhooksCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteWebhooksCreateForbidden), made)

	updated, err := member.SiteWebhooksUpdate(ctx, &siteapi.UpdateWebhookInput{URL: siteapi.NewOptString("https://attacker.example/hook")},
		siteapi.SiteWebhooksUpdateParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteWebhooksUpdateForbidden), updated)

	regenerated, err := member.SiteWebhooksRegenerateSecret(ctx, siteapi.SiteWebhooksRegenerateSecretParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteWebhooksRegenerateSecretForbidden), regenerated)

	tested, err := member.SiteWebhooksTest(ctx, &siteapi.TestWebhookInput{EventType: "contact.created"},
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
	endpoint, ok := read.(*siteapi.SiteWebhookEndpointResource)
	require.True(t, ok, "got %T", read)
	assert.Equal(t, "https://example.com/hook", endpoint.URL)

	// An admin manages them.
	adminList, err := env.SiteActor(t, fixtures.AdminAliceEmail).SiteWebhooksList(ctx, siteapi.SiteWebhooksListParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteWebhooksListOK), adminList)
}
