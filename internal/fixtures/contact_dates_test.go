package fixtures_test

import (
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// keyRow is a row of contact_dates as the fixtures must hold it; its month and day are checked
// against its date by the table.
func keyRow(workspaceID, contactID int64, k datetrigger.Keyed) string {
	return fmt.Sprintf("workspace %d contact %d %s/%s: %s", workspaceID, contactID, k.Field.Kind, k.Field.Key, k.Key)
}

// Fixtures load without the contacts module, which writes a Contact's date keys with it (ADR 0126):
// fixtures/contact_dates.yml holds the key of every date a fixture Contact holds, and nothing else,
// or the date scheduler would never find it.
func TestFixtureContactDatesAreTheKeysOfTheirContacts(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	contacts, err := env.DB.Contact.Query().All(ctx)
	require.NoError(t, err)
	stored, err := env.DB.ContactDate.Query().All(ctx)
	require.NoError(t, err)

	want := lo.FlatMap(contacts, func(c *ent.Contact, _ int) []string {
		return lo.Map(datetrigger.Holder{Birthday: c.Birthday, CustomFields: c.CustomFields}.Keys(), func(k datetrigger.Keyed, _ int) string {
			return keyRow(c.WorkspaceID, c.ID, k)
		})
	})
	got := lo.Map(stored, func(d *ent.ContactDate, _ int) string {
		return keyRow(d.WorkspaceID, d.ContactID, datetrigger.Keyed{Field: datetrigger.Field{Kind: d.Kind, Key: d.Key}, Key: localcalendar.DateOf(d.Date)})
	})

	require.NotEmpty(t, want, "fixture Contacts hold dates")
	assert.ElementsMatch(t, want, got)
}
