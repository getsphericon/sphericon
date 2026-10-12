//go:build e2e

package e2e

import (
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
)

// ImportContacts upserts a Contact per email through the batch endpoint.
func (w *Workspace) ImportContacts(emails ...string) {
	w.t.Helper()
	items := make([]externalapi.UpsertContactInput, len(emails))
	for i, e := range emails {
		items[i] = externalapi.UpsertContactInput{Email: externalapi.NewOptNilEmailAddress(externalapi.EmailAddress(e))}
	}
	res, err := w.api.ContactsBatchUpsert(w.t.Context(), &externalapi.UpsertContactsInput{Contacts: items})
	out := ok[externalapi.UpsertContactsResult](w.t, "import contacts", res, err)
	for _, r := range out.Results {
		require.NotEqual(w.t, externalapi.ContactBatchStatusFailed, r.Status, "import contact #%d: %s", r.Index, r.Error.Value)
	}
}
