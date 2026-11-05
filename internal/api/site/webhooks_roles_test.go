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

// An endpoint carries its signing secret and decides where events are delivered: every
// operation on one, reads included, needs an Owner or an Admin.
func TestSiteWebhooksNeedAnOwnerOrAdmin(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	id := siteapi.EntityId(fixtures.WebhookCodebasicsID)

	member := env.SiteActor(t, fixtures.MemberMaryEmail)

	_, err := member.SiteWebhooksList(ctx, siteapi.SiteWebhooksListParams{WorkspaceId: workspaceID})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)

	_, err = member.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams{WorkspaceId: workspaceID, ID: id})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)

	_, err = member.SiteWebhooksCreate(ctx, &siteapi.CreateWebhookInput{URL: "https://attacker.example/hook"},
		siteapi.SiteWebhooksCreateParams{WorkspaceId: workspaceID})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)

	_, err = member.SiteWebhooksUpdate(ctx, &siteapi.UpdateWebhookInput{URL: siteapi.NewOptString("https://attacker.example/hook")},
		siteapi.SiteWebhooksUpdateParams{WorkspaceId: workspaceID, ID: id})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)

	_, err = member.SiteWebhooksRegenerateSecret(ctx, siteapi.SiteWebhooksRegenerateSecretParams{WorkspaceId: workspaceID, ID: id})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)

	_, err = member.SiteWebhooksTest(ctx, &siteapi.TestWebhookInput{EventType: "contact.created"},
		siteapi.SiteWebhooksTestParams{WorkspaceId: workspaceID, ID: id})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)

	err = member.SiteWebhooksDelete(ctx, siteapi.SiteWebhooksDeleteParams{WorkspaceId: workspaceID, ID: id})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)

	// The refused calls changed nothing: the owner still sees the endpoint, unchanged.
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	endpoint, err := owner.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	assert.Equal(t, fixtures.WebhookCodebasicsURL, endpoint.URL)

	// An admin manages them.
	_, err = env.SiteActor(t, fixtures.AdminAliceEmail).SiteWebhooksList(ctx, siteapi.SiteWebhooksListParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
}
