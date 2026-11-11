//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Each mailbox provider's encoding of the one-click POST opts its recipient out; a scanner that
// fetches the same URL with HEAD and GET opts nobody out.
func TestOneClickUnsubscribeStopsTheNextBroadcast(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t).Ready()
	multipartLeaver, urlEncodedLeaver, scanned := w.NewRecipient(), w.NewRecipient(), w.NewRecipient()
	w.ImportContacts(multipartLeaver, urlEncodedLeaver, scanned)

	first := "First " + uniq()
	w.SendBroadcast(Broadcast{Subject: first, Body: HTML("First issue")})
	toMultipart := w.Inbox.Wait(Match{To: multipartLeaver, Subject: first})
	toURLEncoded := w.Inbox.Wait(Match{To: urlEncodedLeaver, Subject: first})
	toScanned := w.Inbox.Wait(Match{To: scanned, Subject: first})

	// Only what the email itself carries: URL and body from its headers.
	w.PrefetchUnsubscribe(toScanned)
	w.OneClickUnsubscribe(toMultipart, OneClickMultipart)
	w.OneClickUnsubscribe(toURLEncoded, OneClickURLEncoded)
	w.RequireUnsubscribed(multipartLeaver)
	w.RequireUnsubscribed(urlEncodedLeaver)

	second := "Second " + uniq()
	w.SendBroadcast(Broadcast{Subject: second, Body: HTML("Second issue")})
	w.Inbox.Wait(Match{To: scanned, Subject: second}) // the send ran, and the scan opted nobody out
	w.Inbox.RequireNone(Match{To: multipartLeaver, Subject: second})
	w.Inbox.RequireNone(Match{To: urlEncodedLeaver, Subject: second})
	assert.NotEmpty(t, toMultipart.Header("List-Unsubscribe"))
}
