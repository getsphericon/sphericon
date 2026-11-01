package external_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The System events a Contact's Tags record (#897): one only when the Tag set changed.
// Fixtures: Bob has no Tags.
func TestTagChangesRecordSystemEvents(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:read", "contacts:write")
	ctx := context.Background()
	bob := entityIDString(fixtures.ContactBobID)
	vip := map[string]any{"contactId": fixtures.ContactBobID, "tag": "vip"}
	apply := func() {
		t.Helper()
		_, err := c.TagsApply(ctx, &externalapi.ApplyTagInput{Name: "vip"}, externalapi.TagsApplyParams{ContactId: bob})
		require.NoError(t, err)
	}
	remove := func() {
		t.Helper()
		_, err := c.TagsRemove(ctx, externalapi.TagsRemoveParams{ContactId: bob, Name: "vip"})
		require.NoError(t, err)
	}

	apply()
	taggedOnce := env.OutboxCount(t, events.NameContactTagged, vip)
	apply()
	taggedAgain := env.OutboxCount(t, events.NameContactTagged, vip)
	remove()
	untaggedOnce := env.OutboxCount(t, events.NameContactUntagged, vip)
	remove()
	untaggedAgain := env.OutboxCount(t, events.NameContactUntagged, vip)

	assert.Equal(t, 1, taggedOnce, "applying a Tag the Contact lacks records contact.tagged")
	assert.Equal(t, 1, taggedAgain, "applying it again records nothing")
	assert.Equal(t, 1, untaggedOnce, "removing it records contact.untagged")
	assert.Equal(t, 1, untaggedAgain, "removing a Tag the Contact lacks records nothing")
}

// A Contact's update replaces its Custom fields: a field it drops is cleared, one it changes is
// set, one it repeats (a number written another way included) records nothing.
func TestContactUpdateRecordsFieldChanges(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:read", "contacts:write")
	ctx := context.Background()
	bob := entityIDString(fixtures.ContactBobID)
	update := func(fields externalapi.JsonObject) {
		t.Helper()
		res, err := c.ContactsUpdate(ctx, &externalapi.UpdateContactInput{
			CustomFields: externalapi.NewOptNilJsonObject(fields),
		}, externalapi.ContactsUpdateParams{ID: bob})
		require.NoError(t, err)
		require.IsType(t, &externalapi.ContactResource{}, res)
	}

	update(externalapi.JsonObject{"plan": jx.Raw(`"pro"`), "seats": jx.Raw(`4`)})
	update(externalapi.JsonObject{"plan": jx.Raw(`"pro"`), "seats": jx.Raw(`4.0`)})
	update(externalapi.JsonObject{"seats": jx.Raw(`5`)})
	changes := env.FieldChanges(t)

	assert.ElementsMatch(t, []map[string]any{
		{"key": "plan", "value": "pro"},
		{"key": "seats", "value": json.Number("4")},
		{"key": "plan", "value": nil},
		{"key": "seats", "value": json.Number("5")},
	}, changes)
}

// An import brings earlier state: it records no Custom field change (#897), only the Contacts it
// creates.
func TestBatchImportRecordsNoFieldChanges(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:read", "contacts:write")

	res, err := c.ContactsBatchUpsert(context.Background(), &externalapi.UpsertContactsInput{Contacts: []externalapi.UpsertContactInput{
		{
			Email:        externalapi.NewOptNilEmailAddress("imported@example.org"),
			CustomFields: externalapi.NewOptNilJsonObject(externalapi.JsonObject{"plan": jx.Raw(`"pro"`)}),
		},
		{
			Email:        externalapi.NewOptNilEmailAddress(fixtures.ContactBobEmail),
			CustomFields: externalapi.NewOptNilJsonObject(externalapi.JsonObject{"plan": jx.Raw(`"team"`)}),
		},
	}})
	require.NoError(t, err)
	require.IsType(t, &externalapi.UpsertContactsResult{}, res)
	changes := env.FieldChanges(t)
	created := env.OutboxEnvelopes(t, events.NameContactCreated)

	assert.Empty(t, changes)
	assert.Len(t, created, 1, "the import still records the Contact it creates")
}
