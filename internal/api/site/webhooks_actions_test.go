package site_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/webhook"
)

func TestSiteWebhookRegenerateSecretReturnsTheNewSecretAndIsAudited(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	params := siteapi.SiteWebhooksRegenerateSecretParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.WebhookCodebasicsID)}

	res, err := owner.SiteWebhooksRegenerateSecret(ctx, params)
	require.NoError(t, err)
	hook, ok := res.(*siteapi.SiteWebhookEndpointResource)
	require.Truef(t, ok, "got %T", res)
	assert.NotEqual(t, "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw", hook.Secret)

	got, err := owner.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.WebhookCodebasicsID)})
	require.NoError(t, err)
	assert.Equal(t, hook.Secret, got.(*siteapi.SiteWebhookEndpointResource).Secret)

	entries := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, "webhook_endpoint.update")
	require.Len(t, entries, 1)
	assert.Equal(t, siteapi.SiteAuditActorKindUser, entries[0].Actor.Kind)

	other, err := owner.SiteWebhooksRegenerateSecret(ctx, siteapi.SiteWebhooksRegenerateSecretParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.WebhookGlobexID)})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteWebhooksRegenerateSecretNotFound), other)
}

func TestSiteWebhookTestSendReportsTheAnswerAndIsAudited(t *testing.T) {
	ctx := context.Background()
	rcv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusGone) }))
	t.Cleanup(rcv.Close)
	u, err := url.Parse(rcv.URL)
	require.NoError(t, err)
	allow, err := webhook.ParseAllowlist(u.Host)
	require.NoError(t, err)
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.OutboundAllow = allow }))
	_, err = env.DB.Scoped(fixtures.AcmeID).WebhookEndpoint().UpdateOneID(fixtures.WebhookCodebasicsID).SetURL(rcv.URL).Save(ctx)
	require.NoError(t, err)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	params := siteapi.SiteWebhooksTestParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.WebhookCodebasicsID)}

	res, err := owner.SiteWebhooksTest(ctx, &siteapi.SiteTestWebhookInput{EventType: "email.opened"}, params)
	require.NoError(t, err)
	got, ok := res.(*siteapi.SiteWebhookTestResult)
	require.Truef(t, ok, "got %T", res)
	assert.False(t, got.Delivered)
	assert.Equal(t, siteapi.SiteWebhookErrorKindHTTPStatus, got.Kind.Value)
	assert.Equal(t, int32(http.StatusGone), got.Status.Value)

	row, err := env.DB.WebhookEndpoint.Get(ctx, fixtures.WebhookCodebasicsID)
	require.NoError(t, err)
	assert.Nil(t, row.DisabledAt, "a 410 on a test does not disable the endpoint")
	assert.Nil(t, row.ConsecutiveFailures)

	entries := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionWebhookEndpointTest)
	require.Len(t, entries, 1)
	assert.Equal(t, "webhook_endpoint", entries[0].Target.Type)

	blank, err := owner.SiteWebhooksTest(ctx, &siteapi.SiteTestWebhookInput{EventType: ""}, params)
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteWebhooksTestUnprocessableEntity), blank)
}
