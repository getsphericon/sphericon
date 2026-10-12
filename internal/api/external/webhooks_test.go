package external_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExternalWebhooksCRUD(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "webhooks:read", "webhooks:write")
	ctx := context.Background()

	got, err := c.WebhooksGet(ctx, externalapi.WebhooksGetParams{ID: entityIDString(fixtures.WebhookCodebasicsID)})
	require.NoError(t, err)
	hook, ok := got.(*externalapi.WebhookResource)
	require.Truef(t, ok, "got %T", got)
	assert.Equal(t, "https://codebasics.dev/webhooks/sphericon", hook.URL)
	assert.Equal(t, []string{"contact.created", "email.opened", "email.clicked"}, hook.EventTypes)

	bad, err := c.WebhooksCreate(ctx, &externalapi.CreateWebhookInput{URL: "not-a-url"})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.WebhooksCreateUnprocessableEntity{}, bad)

	created, err := c.WebhooksCreate(ctx, &externalapi.CreateWebhookInput{
		URL: "https://example.com/hook", EventTypes: []string{"contact.created"},
	})
	require.NoError(t, err)
	res, ok := created.(*externalapi.WebhookResource)
	require.Truef(t, ok, "got %T", created)
	assert.True(t, res.Enabled)
	assert.Equal(t, []string{"contact.created"}, res.EventTypes)

	// A signing secret was generated and stored encrypted, just not exposed.
	row, err := env.DB.WebhookEndpoint.Query().Where(webhookendpoint.URL("https://example.com/hook")).Only(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, row.SecretEncrypted)

	upd, err := c.WebhooksUpdate(ctx, &externalapi.UpdateWebhookInput{
		Enabled: externalapi.NewOptBool(false), EventTypes: []string{},
	}, externalapi.WebhooksUpdateParams{ID: res.ID})
	require.NoError(t, err)
	updated, ok := upd.(*externalapi.WebhookResource)
	require.Truef(t, ok, "got %T", upd)
	assert.False(t, updated.Enabled)
	assert.Empty(t, updated.EventTypes)
	after, err := env.DB.WebhookEndpoint.Get(ctx, row.ID)
	require.NoError(t, err)
	assert.Equal(t, row.SecretEncrypted, after.SecretEncrypted, "update keeps the secret")

	badUpd, err := c.WebhooksUpdate(ctx, &externalapi.UpdateWebhookInput{URL: externalapi.NewOptString("ftp://x")},
		externalapi.WebhooksUpdateParams{ID: res.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.WebhooksUpdateUnprocessableEntity{}, badUpd)

	del, err := c.WebhooksDelete(ctx, externalapi.WebhooksDeleteParams{ID: res.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.WebhooksDeleteNoContent{}, del)
	missing, err := c.WebhooksGet(ctx, externalapi.WebhooksGetParams{ID: res.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.WebhooksGetNotFound{}, missing)
}

// The signing secret is never part of any /api response: not on list, get,
// create or update, neither in clear nor sealed.
func TestExternalWebhooksNeverReturnSecrets(t *testing.T) {
	env := testhelper.Setup(t)
	token := env.ScopedBearer(t, "webhooks:read", "webhooks:write")
	doer := env.Transport(map[string]string{"Authorization": "Bearer " + token})

	cases := []struct{ name, method, path, body string }{
		{"list", http.MethodGet, "/api/webhooks", ""},
		{"get", http.MethodGet, fmt.Sprintf("/api/webhooks/%d", fixtures.WebhookCodebasicsID), ""},
		{"create", http.MethodPost, "/api/webhooks", `{"url":"https://example.com/h"}`},
		{"update", http.MethodPut, fmt.Sprintf("/api/webhooks/%d", fixtures.WebhookCodebasicsID), `{"enabled":false}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), tc.method, "http://local"+tc.path, strings.NewReader(tc.body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			resp, err := doer.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			raw, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Less(t, resp.StatusCode, 300, string(raw))
			assert.NotContains(t, strings.ToLower(string(raw)), "secret")
			assert.NotContains(t, string(raw), "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw")
		})
	}
}

func TestExternalWebhooksScopes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	res, err := env.ExternalAnonymous(t).WebhooksList(ctx, externalapi.WebhooksListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.WebhooksListUnauthorized{}, res)

	// Template scopes do not grant webhooks.
	other := env.ExternalScoped(t, "templates:read", "templates:write")
	l, err := other.WebhooksList(ctx, externalapi.WebhooksListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.WebhooksListUnauthorized{}, l)

	ro := env.ExternalScoped(t, "webhooks:read")
	g, err := ro.WebhooksGet(ctx, externalapi.WebhooksGetParams{ID: entityIDString(fixtures.WebhookCodebasicsID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.WebhookResource{}, g)
	cr, err := ro.WebhooksCreate(ctx, &externalapi.CreateWebhookInput{URL: "https://example.com/x"})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.WebhooksCreateUnauthorized{}, cr)
	up, err := ro.WebhooksUpdate(ctx, &externalapi.UpdateWebhookInput{}, externalapi.WebhooksUpdateParams{ID: entityIDString(fixtures.WebhookCodebasicsID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.WebhooksUpdateUnauthorized{}, up)
	de, err := ro.WebhooksDelete(ctx, externalapi.WebhooksDeleteParams{ID: entityIDString(fixtures.WebhookCodebasicsID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.WebhooksDeleteUnauthorized{}, de)
}

func TestExternalWebhooksAreWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	id := entityIDString(fixtures.WebhookGlobexID)

	c := env.ExternalScoped(t, "webhooks:read", "webhooks:write")
	list, err := c.WebhooksList(ctx, externalapi.WebhooksListParams{})
	require.NoError(t, err)
	for _, it := range list.(*externalapi.WebhooksListOK).Items {
		assert.NotEqual(t, fixtures.WebhookGlobexURL, it.URL)
	}
	g, err := c.WebhooksGet(ctx, externalapi.WebhooksGetParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.WebhooksGetNotFound{}, g)
	u, err := c.WebhooksUpdate(ctx, &externalapi.UpdateWebhookInput{Enabled: externalapi.NewOptBool(false)}, externalapi.WebhooksUpdateParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.WebhooksUpdateNotFound{}, u)
	d, err := c.WebhooksDelete(ctx, externalapi.WebhooksDeleteParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.WebhooksDeleteNotFound{}, d)
}

// audit.entry is selectable on a Webhook endpoint only with an Enterprise license,
// through /api as through /site (ADR 0022).
func TestExternalWebhookSelectsAuditEntryOnlyUnderLicense(t *testing.T) {
	ctx := context.Background()
	auditTypes := []string{"audit.entry"}

	licensed := testhelper.Setup(t).ExternalScoped(t, "webhooks:write")
	res, err := licensed.WebhooksCreate(ctx, &externalapi.CreateWebhookInput{URL: "https://x.test/audit", EventTypes: auditTypes})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.WebhookResource{}, res)

	env := testhelper.Setup(t, testhelper.WithoutLicense())
	c := env.ExternalScoped(t, "webhooks:write")
	res, err = c.WebhooksCreate(ctx, &externalapi.CreateWebhookInput{URL: "https://x.test/audit", EventTypes: auditTypes})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.WebhooksCreateUnprocessableEntity{}, res)

	upd, err := c.WebhooksUpdate(ctx, &externalapi.UpdateWebhookInput{EventTypes: auditTypes},
		externalapi.WebhooksUpdateParams{ID: entityIDString(fixtures.WebhookCodebasicsID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.WebhooksUpdateUnprocessableEntity{}, upd)
}
