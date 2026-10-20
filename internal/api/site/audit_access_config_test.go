package site_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Sending domain, Integration, Webhook endpoint and API token are audited through the
// scoped client, and no secret (DKIM private key, Integration credentials, webhook
// signing secret, token hash or its one-time value) reaches any entry.
func TestAccessAndConfigEntitiesAreAuditedWithoutSecrets(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	slug := fixtures.AcmeSlug

	domain, err := c.SiteSendingDomainsCreate(ctx, &siteapi.SiteCreateSendingDomainInput{Domain: "audited.example.com"},
		siteapi.SiteSendingDomainsCreateParams{Slug: slug})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteSendingDomainResource{}, domain)

	integration, err := c.SiteIntegrationsCreate(ctx, &siteapi.SiteCreateIntegrationInput{
		Name:   "Audited SMTP",
		Config: smtpInput("smtp.example.com", "integration-pass-9"),
	}, siteapi.SiteIntegrationsCreateParams{Slug: slug})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteIntegrationResource{}, integration)

	hook, err := c.SiteWebhooksCreate(ctx, &siteapi.SiteCreateWebhookEndpointInput{URL: "https://user:pw-9@example.com/audited?token=url-secret-7#frag"},
		siteapi.SiteWebhooksCreateParams{Slug: slug})
	require.NoError(t, err)
	hookResp, ok := hook.(*siteapi.SiteWebhookEndpointResource)
	require.True(t, ok)

	token, err := c.SiteTokensCreate(ctx, &siteapi.SiteCreateTokenInput{Name: "audited", Scopes: []string{"contacts:read"}},
		siteapi.SiteTokensCreateParams{Slug: slug})
	require.NoError(t, err)
	tokenResp, tok := token.(*siteapi.SiteCreateTokenResponse)
	require.True(t, tok)

	actions := map[string]*events.AuditEntry{}
	var all strings.Builder
	for _, ev := range env.OutboxEvents(t, events.NameAuditEntry) {
		e := ev.(*events.AuditEntry)
		actions[e.Action] = e
		b, merr := json.Marshal(e)
		require.NoError(t, merr)
		all.Write(b)
	}
	for _, action := range []string{"sending_domain.create", "integration.create", "webhook_endpoint.create", "api_token.create"} {
		assert.Contains(t, actions, action)
	}
	assert.Equal(t, "changed", actions["sending_domain.create"].Diff["dkim_private_key_encrypted"])
	assert.Equal(t, "changed", actions["integration.create"].Diff["config_encrypted"])
	assert.Equal(t, "changed", actions["webhook_endpoint.create"].Diff["secret_encrypted"])
	assert.Equal(t, "changed", actions["api_token.create"].Diff["secret_hash"])
	assert.Equal(t, "changed", actions["webhook_endpoint.create"].Diff["url"], "the URL may carry a secret")
	assert.Equal(t, "https://example.com/audited", actions["webhook_endpoint.create"].TargetName, "scheme, host and path only")
	assert.Equal(t, "audited.example.com", actions["sending_domain.create"].TargetName)
	assert.Equal(t, "Audited SMTP", actions["integration.create"].TargetName)

	log := all.String()
	assert.NotContains(t, log, "integration-pass-9")
	assert.NotContains(t, log, "BEGIN")
	assert.NotContains(t, log, "url-secret-7")
	assert.NotContains(t, log, "pw-9")
	assert.NotContains(t, log, tokenResp.Token)
	assert.NotContains(t, log, hookResp.Secret)
}

// Workspace settings changes are recorded with the changed fields, naming the User.
func TestWorkspaceSettingsChangeIsAudited(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := c.SiteWorkspacesUpdate(context.Background(),
		&siteapi.SiteUpdateWorkspaceInput{Name: "Acme Renamed", PostalAddress: siteapi.NewOptString("1 Main St")},
		siteapi.SiteWorkspacesUpdateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)

	got := env.OutboxEvents(t, events.NameAuditEntry)
	require.Len(t, got, 1)
	e := got[0].(*events.AuditEntry)
	assert.Equal(t, events.ActionWorkspaceUpdate, e.Action)
	assert.Equal(t, events.ActorUser, e.Actor.Kind)
	assert.Equal(t, "Acme Renamed", e.TargetName)
	assert.Equal(t, map[string]any{"from": fixtures.AcmeName, "to": "Acme Renamed"}, e.Diff["name"])
	assert.Contains(t, e.Diff, "postal_address")
}

// Revoking a token is a bulk update by predicate; it still records one entry per row,
// with the revocation time and no secret.
func TestApiTokenRevocationIsAudited(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	created, err := c.SiteTokensCreate(ctx, &siteapi.SiteCreateTokenInput{Name: "to-revoke", Scopes: []string{"contacts:read"}},
		siteapi.SiteTokensCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	resp, ok := created.(*siteapi.SiteCreateTokenResponse)
	require.True(t, ok)

	del, err := c.SiteTokensDelete(ctx, siteapi.SiteTokensDeleteParams{Slug: fixtures.AcmeSlug, ID: resp.Resource.ID})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteTokensDeleteNoContent{}, del)

	updates := lo.FilterMap(env.OutboxEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) (*events.AuditEntry, bool) {
		e := ev.(*events.AuditEntry)
		return e, e.Action == "api_token.update"
	})
	require.NotEmpty(t, updates, "revocation emits api_token.update")
	revoke := updates[len(updates)-1]
	assert.Equal(t, "to-revoke", revoke.TargetName)
	assert.Contains(t, revoke.Diff, "revoked_at")
	assert.NotContains(t, revoke.Diff, "secret_hash")
}
