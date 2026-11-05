//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"regexp"

	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
)

// SubscribeWebhook adds a Webhook endpoint to the Workspace at a Hook of the test's own,
// selecting eventTypes (every customer-facing event when there are none), and returns the Hook.
// The endpoint's signing secret is shown only in the app, never through /api, so the Hook
// checks that each delivery is signed, not with what.
func (w *Workspace) SubscribeWebhook(eventTypes ...string) *Hook {
	w.t.Helper()
	hook := w.NewHook(false)
	hook.signedBySomeone = true
	res, err := w.api.WebhooksCreate(w.t.Context(), &externalapi.CreateWebhookInput{URL: hook.URL, EventTypes: eventTypes})
	ok[externalapi.WebhookResource](w.t, "create webhook", res, err)
	return hook
}

// trackedLink finds the first tracked link of an email's HTML.
var trackedLink = regexp.MustCompile(`href="(https?://[^"]*/e/c/[^"]+)"`)

// ClickLink follows the first tracked link of an email the way a person's browser does and
// requires the redirect to the link's destination. The click is recorded asynchronously, so a
// scenario observes its effect (a webhook delivery) with Hook.Wait.
func (w *Workspace) ClickLink(msg Message) {
	w.t.Helper()
	m := trackedLink.FindStringSubmatch(msg.HTML)
	require.NotNil(w.t, m, "the email %q carries a tracked link", msg.Subject)
	req, err := http.NewRequestWithContext(w.t.Context(), http.MethodGet, m[1], http.NoBody)
	require.NoError(w.t, err)
	// A person's browser, not Go's default agent: that one is machine engagement (ADR 0053).
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	require.NoError(w.t, err)
	_, err = io.Copy(io.Discard, res.Body)
	require.NoError(w.t, err)
	require.NoError(w.t, res.Body.Close())
	require.Equal(w.t, http.StatusFound, res.StatusCode, "tracked link %s", m[1])
}
