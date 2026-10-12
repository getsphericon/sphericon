//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/eligibility"
)

// OneClickUnsubscribe performs the RFC 8058 one-click request exactly as a mailbox
// provider would: it uses only the https URL from the message's List-Unsubscribe
// header and the body from List-Unsubscribe-Post, and requires success (2xx).
func (w *Workspace) OneClickUnsubscribe(msg Message) {
	w.t.Helper()
	target := msg.UnsubscribeURL()
	require.NotEmpty(w.t, target, "List-Unsubscribe carries an http(s) URL, got %q", msg.Header("List-Unsubscribe"))
	body := msg.OneClickBody()
	require.Equal(w.t, "List-Unsubscribe=One-Click", body, "List-Unsubscribe-Post announces one-click")

	req, err := http.NewRequestWithContext(w.t.Context(), http.MethodPost, target, strings.NewReader(body))
	require.NoError(w.t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	require.NoError(w.t, err)
	out, err := io.ReadAll(res.Body)
	require.NoError(w.t, err)
	require.NoError(w.t, res.Body.Close())
	require.Truef(w.t, res.StatusCode/100 == 2, "one-click POST to %s answered %d: %s", target, res.StatusCode, out)
}

// RequireUnsubscribed asserts, through the external API, that the Contact with this
// email has opted out of Broadcasts. The API reads it from the Contact's data export
// (its opt-outs); the opt-out is recorded asynchronously, so it is polled.
func (w *Workspace) RequireUnsubscribed(email string) {
	w.t.Helper()
	ctx := w.t.Context()
	require.Eventually(w.t, func() bool {
		res, err := w.api.ContactsExport(ctx, externalapi.ContactsExportParams{Email: externalapi.NewOptEmailAddress(externalapi.EmailAddress(email))})
		if err != nil {
			return false
		}
		dl, isDL := res.(*externalapi.ContactsExportOKApplicationOctetStreamHeaders)
		if !isDL {
			return false
		}
		raw, err := io.ReadAll(dl.Response)
		if err != nil {
			return false
		}
		var doc externalapi.ContactExportDocument
		if err := doc.Decode(jx.DecodeBytes(raw)); err != nil {
			return false
		}
		for _, u := range doc.Unsubscribes {
			if strings.EqualFold(u.Destination, email) && u.SendingSource == eligibility.SourceBroadcasts {
				return true
			}
		}
		return false
	}, EmailTimeout, 100*time.Millisecond, "%s never read as unsubscribed from broadcasts", email)
}
