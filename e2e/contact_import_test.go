//go:build e2e

package e2e

import (
	"testing"

	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// A CSV import brings its Contacts in as history (ADR 0138): they exist with the file's Tags and the
// Tag for every row, their source is `import`, and an Automation on contact.created sends them
// nothing, while a Contact the API creates afterwards gets its welcome.
func TestACSVImportBringsContactsInAndStartsNothing(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t).Ready()
	imported, other, signedUp := w.NewRecipient(), w.NewRecipient(), w.NewRecipient()
	w.PublishAutomation(Automation{
		TriggerEvent: systemevent.ContactCreated,
		Subject:      "Welcome aboard",
		Body:         HTML("Glad to have you"),
	})

	w.ImportCSV("email,first_name,tags\n"+imported+",Ann,\"vip, newsletter\"\n"+other+",Bob,\n", "From the old platform")

	w.RequireImported(imported, "vip", "newsletter", "From the old platform")
	w.RequireImported(other, "From the old platform")
	// A Contact the API creates enrolls: once its welcome arrives, the imported Contacts' would have.
	w.ImportContacts(signedUp)
	w.Inbox.Wait(Match{To: signedUp, Subject: "Welcome aboard"})
	w.Inbox.RequireNone(Match{To: imported})
	w.Inbox.RequireNone(Match{To: other})
}
