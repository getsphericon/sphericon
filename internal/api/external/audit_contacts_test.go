package external_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A batch upsert is an import: ONE contact.import entry (counts only), none per row.
func TestExternalBatchUpsertRecordsOneImportEntry(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalAnchor(t)

	res, err := c.ContactsBatchUpsert(context.Background(), &externalapi.UpsertContactsInput{Contacts: []externalapi.UpsertContactInput{
		{Email: externalapi.NewOptNilEmailAddress("import.one@example.org")},
		{Email: externalapi.NewOptNilEmailAddress("import.two@example.org")},
		{Email: externalapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail)},
		{FirstName: externalapi.NewOptNilString("no identity")},
	}})
	require.NoError(t, err)
	require.IsType(t, &externalapi.UpsertContactsResult{}, res)

	got := env.OutboxEvents(t, events.NameAuditEntry)
	require.Len(t, got, 1)
	e := got[0].(*events.AuditEntry)
	assert.Equal(t, events.ActionContactImport, e.Action)
	assert.Equal(t, events.ActorAPIToken, e.Actor.Kind)
	assert.Empty(t, e.TargetID)
	raw, err := json.Marshal(e)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "example.org")
	assert.NotContains(t, string(raw), fixtures.ContactAliceEmail)
	assert.EqualValues(t, 2, e.Diff["created"])
	assert.EqualValues(t, 1, e.Diff["updated"])
	assert.EqualValues(t, 1, e.Diff["failed"])
}

// The same Contact edit through /api is audited under the token with names only, and
// deleting the Contact (erasure) leaves the entry as it was: it holds no personal data.
func TestExternalContactEditIsAuditedWithNamesOnly(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write", "contacts:erase")
	ctx := context.Background()

	_, err := c.ContactsUpdate(ctx, &externalapi.UpdateContactInput{FirstName: externalapi.NewOptNilString("Renamed")},
		externalapi.ContactsUpdateParams{ID: entityIDString(fixtures.ContactBobID)})
	require.NoError(t, err)
	_, err = c.ContactsDelete(ctx, externalapi.ContactsDeleteParams{ID: entityIDString(fixtures.ContactBobID)})
	require.NoError(t, err)

	got := env.OutboxEvents(t, events.NameAuditEntry)
	require.Len(t, got, 2)
	for _, ev := range got {
		e := ev.(*events.AuditEntry)
		assert.Equal(t, events.ActorAPIToken, e.Actor.Kind)
		assert.Equal(t, strconv.Itoa(fixtures.ContactBobID), e.TargetID)
		assert.Empty(t, e.TargetName)
		raw, err := json.Marshal(e)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), fixtures.ContactBobEmail)
		assert.NotContains(t, string(raw), "Renamed")
		assert.NotContains(t, string(raw), "Bob")
	}
	assert.Equal(t, map[string]any{"first_name": "changed"}, got[0].(*events.AuditEntry).Diff)
}
