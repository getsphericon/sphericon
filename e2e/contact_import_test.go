//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"

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

// With "treat them as new sign-ups" ticked, a CSV import's Contacts start what a sign-up starts
// (ADR 0138): an Automation on contact.created sends its mail to an imported Contact.
func TestACSVImportOfNewSignUpsStartsTheirAutomations(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t).Ready()
	imported := w.NewRecipient()
	w.PublishAutomation(Automation{
		TriggerEvent: systemevent.ContactCreated,
		Subject:      "Welcome aboard",
		Body:         HTML("Glad to have you"),
	})

	w.ImportCSVAsNewSignUps("email,first_name\n"+imported+",Ann\n", "Signed up elsewhere")

	w.RequireImported(imported, "Signed up elsewhere")
	w.Inbox.Wait(Match{To: imported, Subject: "Welcome aboard"})
}

// A file whose status column says one address unsubscribed on the old platform brings that
// refusal (ADR 0138): a Broadcast to everyone reaches the others and not that address.
func TestAnImportedUnsubscribeKeepsABroadcastAway(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t).Ready()
	left, stayed := w.NewRecipient(), w.NewRecipient()

	done := w.ImportRefusals("email,status\n"+left+",Unsubscribed\n"+stayed+",subscribed\n", 1, "unsubscribed")
	w.SendBroadcast(Broadcast{Subject: "Spring news", Body: HTML("What is new this spring")})

	require.Equal(t, int32(1), done.Counts.OptedOut)
	w.Inbox.Wait(Match{To: stayed, Subject: "Spring news"})
	w.Inbox.RequireNone(Match{To: left})
}

// A Workspace's export imports into another Workspace as it is (ADR 0138): the mapping the app
// proposes from the export's headers is saved with only the consent's wording and brand given, and brings the Contacts with their Tags and
// their refusals, so a Broadcast there reaches the others and not the address that opted out.
func TestAnExportImportsIntoAnotherWorkspaceAsItIs(t *testing.T) {
	t.Parallel()
	from := env.NewWorkspace(t)
	to := env.NewWorkspace(t).Ready()
	left, stayed := from.NewRecipient(), from.NewRecipient()
	from.ImportRefusals("email,status,tags\n"+left+",unsubscribed,vip\n"+stayed+",subscribed,vip\n", 1, "unsubscribed")

	done := to.ImportCSVAsProposed(from.ExportCSV())
	to.SendBroadcast(Broadcast{Subject: "Moved house", Body: HTML("We moved to a new platform")})

	require.Equal(t, int32(1), done.Counts.OptedOut)
	to.RequireImported(stayed, "vip")
	to.RequireImported(left, "vip")
	to.Inbox.Wait(Match{To: stayed, Subject: "Moved house"})
	to.Inbox.RequireNone(Match{To: left})
}
