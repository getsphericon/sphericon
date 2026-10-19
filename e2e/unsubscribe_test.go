//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOneClickUnsubscribeStopsTheNextBroadcast(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t).Ready()
	leaver, stayer := w.NewRecipient(), w.NewRecipient()
	w.ImportContacts(leaver, stayer)

	first := "First " + uniq()
	w.SendBroadcast(Broadcast{Subject: first, Body: HTML("First issue")})
	delivered := w.Inbox.Wait(Match{To: leaver, Subject: first})
	w.Inbox.Wait(Match{To: stayer, Subject: first})

	// Only what the email itself carries: URL and body from its headers.
	w.OneClickUnsubscribe(delivered)
	w.RequireUnsubscribed(leaver)

	second := "Second " + uniq()
	w.SendBroadcast(Broadcast{Subject: second, Body: HTML("Second issue")})
	w.Inbox.Wait(Match{To: stayer, Subject: second}) // the send ran; now absence is meaningful
	w.Inbox.RequireNone(Match{To: leaver, Subject: second})
	assert.NotEmpty(t, delivered.Header("List-Unsubscribe"))
}
