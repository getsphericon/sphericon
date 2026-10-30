package external_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/webhook"
)

func TestExternalWebhookRegenerateSecretReplacesItAndNeverReturnsIt(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	id := entityIDString(fixtures.WebhookCodebasicsID)
	before, err := env.DB.WebhookEndpoint.Get(ctx, fixtures.WebhookCodebasicsID)
	require.NoError(t, err)

	denied, err := env.ExternalScoped(t, "webhooks:read", "webhooks:write").WebhooksRegenerateSecret(ctx, externalapi.WebhooksRegenerateSecretParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, new(externalapi.WebhooksRegenerateSecretForbidden), denied)

	res, err := env.ExternalScoped(t, "webhooks:regenerate").WebhooksRegenerateSecret(ctx, externalapi.WebhooksRegenerateSecretParams{ID: id})
	require.NoError(t, err)
	hook, ok := res.(*externalapi.WebhookResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, "https://codebasics.dev/webhooks/sphericon", hook.URL)

	after, err := env.DB.WebhookEndpoint.Get(ctx, fixtures.WebhookCodebasicsID)
	require.NoError(t, err)
	assert.NotEqual(t, before.SecretEncrypted, after.SecretEncrypted)

	missing, err := env.ExternalScoped(t, "webhooks:regenerate").WebhooksRegenerateSecret(ctx, externalapi.WebhooksRegenerateSecretParams{ID: entityIDString(fixtures.WebhookGlobexID)})
	require.NoError(t, err)
	assert.IsType(t, new(externalapi.WebhooksRegenerateSecretNotFound), missing)
}

func TestExternalWebhookTestSendReportsTheReceiversAnswer(t *testing.T) {
	ctx := context.Background()
	status := http.StatusAccepted
	rcv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
	t.Cleanup(rcv.Close)
	u, err := url.Parse(rcv.URL)
	require.NoError(t, err)
	allow, err := webhook.ParseAllowlist(u.Host)
	require.NoError(t, err)
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.OutboundAllow = allow }))
	_, err = env.DB.Scoped(fixtures.AcmeID).WebhookEndpoint().UpdateOneID(fixtures.WebhookCodebasicsFailingID).SetURL(rcv.URL).Save(ctx)
	require.NoError(t, err)
	before, err := env.DB.WebhookEndpoint.Get(ctx, fixtures.WebhookCodebasicsFailingID)
	require.NoError(t, err)
	params := externalapi.WebhooksTestParams{ID: entityIDString(fixtures.WebhookCodebasicsFailingID)}
	input := &externalapi.TestWebhookInput{EventType: "contact.created"}

	denied, err := env.ExternalScoped(t, "webhooks:write").WebhooksTest(ctx, input, params)
	require.NoError(t, err)
	assert.IsType(t, new(externalapi.WebhooksTestForbidden), denied)

	c := env.ExternalScoped(t, "webhooks:test")
	res, err := c.WebhooksTest(ctx, input, params)
	require.NoError(t, err)
	ok, isResult := res.(*externalapi.WebhookTestResult)
	require.Truef(t, isResult, "got %T", res)
	assert.True(t, ok.Delivered)
	assert.Equal(t, int32(http.StatusAccepted), ok.Status.Value)

	status = http.StatusServiceUnavailable
	res, err = c.WebhooksTest(ctx, input, params)
	require.NoError(t, err)
	failed := res.(*externalapi.WebhookTestResult)
	assert.False(t, failed.Delivered)
	assert.Equal(t, externalapi.WebhookErrorKindHTTPStatus, failed.Kind.Value)
	assert.Equal(t, int32(http.StatusServiceUnavailable), failed.Status.Value)

	after, err := env.DB.WebhookEndpoint.Get(ctx, fixtures.WebhookCodebasicsFailingID)
	require.NoError(t, err)
	assert.Equal(t, before.ConsecutiveFailures, after.ConsecutiveFailures, "a test never touches the failure state")

	blank, err := c.WebhooksTest(ctx, &externalapi.TestWebhookInput{EventType: " "}, params)
	require.NoError(t, err)
	assert.IsType(t, new(externalapi.WebhooksTestUnprocessableEntity), blank)
}

func TestExternalWebhookActionsAreAuditedAsTheAPIToken(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "webhooks:regenerate", "webhooks:test")
	id := entityIDString(fixtures.WebhookCodebasicsID)

	_, err := c.WebhooksRegenerateSecret(ctx, externalapi.WebhooksRegenerateSecretParams{ID: id})
	require.NoError(t, err)
	_, err = c.WebhooksTest(ctx, &externalapi.TestWebhookInput{EventType: "contact.created"}, externalapi.WebhooksTestParams{ID: id})
	require.NoError(t, err)

	actions := map[string]*events.AuditEntry{}
	for _, ev := range env.OutboxEvents(t, events.NameAuditEntry) {
		e := ev.(*events.AuditEntry)
		actions[e.Action] = e
	}
	require.Contains(t, actions, "webhook_endpoint.update")
	assert.Equal(t, "api_token", actions["webhook_endpoint.update"].Actor.Kind)
	assert.Equal(t, "changed", actions["webhook_endpoint.update"].Diff["secret_encrypted"])
	require.Contains(t, actions, events.ActionWebhookEndpointTest)
	assert.Equal(t, "api_token", actions[events.ActionWebhookEndpointTest].Actor.Kind)
}
