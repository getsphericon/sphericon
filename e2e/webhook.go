//go:build e2e

package e2e

import (
	"net/http"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// SubscribeWebhook adds a Webhook endpoint to the Workspace at a Hook of the test's own,
// selecting eventTypes (every customer-facing event when there are none), and returns the Hook.
// The endpoint's signing secret is never shown through /api: the Hook takes it from the Webhooks
// settings of the app, where the User copies it, and verifies every delivery with it.
func (w *Workspace) SubscribeWebhook(eventTypes ...string) *Hook {
	w.t.Helper()
	// The endpoint's secret exists only once the endpoint does, so the Hook gets it after.
	hook := w.NewHook(false)
	res, err := w.api.WebhooksCreate(w.t.Context(), &externalapi.CreateWebhookInput{URL: hook.URL, EventTypes: eventTypes})
	ok[externalapi.WebhookResource](w.t, "create webhook", res, err)
	hook.Secret = w.webhookSecret(hook.URL)
	return hook
}

// webhookSecret is the signing secret the Webhooks settings show for the endpoint at endpointURL.
func (w *Workspace) webhookSecret(endpointURL string) string {
	w.t.Helper()
	listed, err := w.site.SiteWebhooksList(w.t.Context(), siteapi.SiteWebhooksListParams{WorkspaceId: siteapi.WorkspaceId(w.publicID)})
	page := ok[siteapi.SiteWebhooksListOK](w.t, "list webhooks", listed, err)
	endpoint, found := lo.Find(page.Items, func(e siteapi.SiteWebhookEndpointResource) bool { return e.URL == endpointURL })
	require.True(w.t, found, "the settings list the endpoint at %s", endpointURL)
	require.NotEmpty(w.t, endpoint.Secret, "the settings show the signing secret of %s", endpointURL)
	return endpoint.Secret
}

// ClickLink follows the first tracked link of an email the way a person's browser does and
// requires the redirect to target, the destination the link was written with. The click is
// recorded asynchronously, so a scenario observes its effect (a webhook delivery) with Hook.Wait.
func (w *Workspace) ClickLink(msg Message, target string) {
	w.t.Helper()
	link := msg.Link("/e/c/")
	require.NotEmpty(w.t, link, "the email %q carries a tracked link", msg.Subject)
	destination := w.openLink(link, http.StatusFound)
	got := destination.String()
	require.Equal(w.t, target, got, "the tracked link %s leads to its destination", link)
}
