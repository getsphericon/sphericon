//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Double opt-in (ADR 0013): in a Workspace that requires confirmed opt-in, an imported Contact gets
// no Broadcast until it confirms; the confirmation email the API asks for reaches it with no
// unsubscribe header, and following its link and pressing Confirm makes the next Broadcast reach it.
func TestDoubleOptInConfirmationMakesTheContactMailable(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t).Ready()
	confirmSubject := "Confirm " + uniq()
	w.RequireConfirmedOptIn(confirmSubject)
	joiner, confirmed := w.NewRecipient(), w.NewRecipient()
	w.ImportContacts(joiner, confirmed)

	w.RequestConfirmation(confirmed)
	mail := w.Inbox.Wait(Match{To: confirmed, Subject: confirmSubject})
	assert.Empty(t, mail.Header("List-Unsubscribe"), "a confirmation email is no marketing")
	w.ConfirmFromEmail(mail)

	issue := "Issue " + uniq()
	w.SendBroadcast(Broadcast{Subject: issue, Body: HTML("The first issue")})
	w.Inbox.Wait(Match{To: confirmed, Subject: issue})
	w.Inbox.RequireNone(Match{To: joiner, Subject: issue})
}
