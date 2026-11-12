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

func TestSiteWebhooksCRUD(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	// Invalid URL is rejected.
	_, err := c.SiteWebhooksCreate(ctx, &siteapi.CreateWebhookInput{URL: "not-a-url"},
		siteapi.SiteWebhooksCreateParams{WorkspaceId: workspaceID})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

	// Create with an event filter.
	created, err := c.SiteWebhooksCreate(ctx, &siteapi.CreateWebhookInput{
		URL:        "https://example.com/hook",
		EventTypes: []string{"contact.created"},
	}, siteapi.SiteWebhooksCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	res := created
	assert.Equal(t, "https://example.com/hook", res.URL)
	assert.Equal(t, []string{"contact.created"}, res.EventTypes)
	assert.True(t, res.Enabled)
	assert.NotEmpty(t, res.Secret, "signing secret is returned for verification")

	// Fetch it back by id (selection by key).
	gotRes, err := c.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	assert.Equal(t, res.ID, gotRes.ID)

	// Update: disable and broaden to all events.
	updRes, err := c.SiteWebhooksUpdate(ctx, &siteapi.UpdateWebhookInput{Revision: siteapi.Revision(env.WebhookRevision(ctx, t, int64(res.ID))),
		Enabled:    siteapi.NewOptBool(false),
		EventTypes: []string{},
	}, siteapi.SiteWebhooksUpdateParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	assert.False(t, updRes.Enabled)
	assert.Empty(t, updRes.EventTypes)
	assert.Equal(t, res.Secret, updRes.Secret, "secret is stable across updates")

	// Delete; a fetch by id then resolves to 404.
	err = c.SiteWebhooksDelete(ctx, siteapi.SiteWebhooksDeleteParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)

	_, err = c.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams{WorkspaceId: workspaceID, ID: res.ID})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}
