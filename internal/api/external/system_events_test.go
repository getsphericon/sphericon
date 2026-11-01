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
	tagged := func() int {
		return env.OutboxCount(t, "contact.tagged", map[string]any{"contactId": fixtures.ContactBobID, "tag": "vip"})
	}
	untagged := func() int {
		return env.OutboxCount(t, "contact.untagged", map[string]any{"contactId": fixtures.ContactBobID, "tag": "vip"})
	}

	_, err := c.TagsApply(ctx, &externalapi.ApplyTagInput{Name: "vip"}, externalapi.TagsApplyParams{ContactId: bob})
	require.NoError(t, err)
	assert.Equal(t, 1, tagged(), "applying a Tag the Contact lacks records contact.tagged")

	_, err = c.TagsApply(ctx, &externalapi.ApplyTagInput{Name: "vip"}, externalapi.TagsApplyParams{ContactId: bob})
	require.NoError(t, err)
	assert.Equal(t, 1, tagged(), "applying it again records nothing")

	_, err = c.TagsRemove(ctx, externalapi.TagsRemoveParams{ContactId: bob, Name: "vip"})
	require.NoError(t, err)
	assert.Equal(t, 1, untagged(), "removing it records contact.untagged")

	_, err = c.TagsRemove(ctx, externalapi.TagsRemoveParams{ContactId: bob, Name: "vip"})
	require.NoError(t, err)
	assert.Equal(t, 1, untagged(), "removing a Tag the Contact lacks records nothing")
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
	fieldChanges := func() []map[string]any {
		messages := env.Outbox(t, events.NameContactFieldChanged)
		out := make([]map[string]any, 0, len(messages))
		for i := range messages {
			out = append(out, map[string]any{"key": messages[i].Data["key"], "value": messages[i].Data["value"]})
		}
		return out
	}

	update(externalapi.JsonObject{"plan": jx.Raw(`"pro"`), "seats": jx.Raw(`4`)})
	update(externalapi.JsonObject{"plan": jx.Raw(`"pro"`), "seats": jx.Raw(`4.0`)})
	update(externalapi.JsonObject{"seats": jx.Raw(`5`)})

	assert.ElementsMatch(t, []map[string]any{
		{"key": "plan", "value": "pro"},
		{"key": "seats", "value": json.Number("4")},
		{"key": "plan", "value": nil},
		{"key": "seats", "value": json.Number("5")},
	}, fieldChanges())
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

	assert.Empty(t, env.OutboxEnvelopes(t, events.NameContactFieldChanged))
	assert.Len(t, env.OutboxEnvelopes(t, events.NameContactCreated), 1, "the import still records the Contact it creates")
}
