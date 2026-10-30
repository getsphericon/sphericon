package external_test

import (
	"context"
	"errors"
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/hook"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestExternalContactsUpdatePersistsAttributesAndCustomFields(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "contacts:read", "contacts:write")

	res, err := c.ContactsUpdate(ctx, &externalapi.UpdateContactInput{
		FirstName: externalapi.NewOptNilString("Alicia"),
		CustomFields: externalapi.NewOptNilUpdateContactInputCustomFields(externalapi.UpdateContactInputCustomFields{
			"plan": jx.Raw(`"pro"`), "seats": jx.Raw(`4`),
		}),
	}, externalapi.ContactsUpdateParams{ID: entityIDString(fixtures.ContactAliceID)})
	require.NoError(t, err)
	got, ok := res.(*externalapi.ContactResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, "Alicia", got.FirstName.Value)

	stored, err := env.DB.Contact.Get(ctx, fixtures.ContactAliceID)
	require.NoError(t, err)
	assert.Equal(t, "Alicia", *stored.FirstName)
	assert.Equal(t, "pro", stored.CustomFields["plan"])
	assert.EqualValues(t, 4, stored.CustomFields["seats"])
	assert.Equal(t, fixtures.ContactAliceEmail, *stored.Email, "an absent email is left alone")
	n, err := env.DB.CustomField.Query().Where(customfield.WorkspaceID(fixtures.AcmeID), customfield.Key("seats"), customfield.TypeEQ(customfield.TypeNumber)).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the field is declared by use, typed")
}

// Isolated: the unique violation aborts this test's transaction.
func TestExternalContactsUpdateConflictNamesTheField(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")

	res, err := c.ContactsUpdate(context.Background(), &externalapi.UpdateContactInput{
		Email: externalapi.NewOptNilEmailAddress(fixtures.ContactBobEmail),
	}, externalapi.ContactsUpdateParams{ID: entityIDString(fixtures.ContactAliceID)})
	require.NoError(t, err)
	conflict, ok := res.(*externalapi.ContactsUpdateConflict)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, conflict.Errors.Value, "email")
}

func TestExternalContactsAreWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	foreign := entityIDString(fixtures.ContactGlobexID)
	c := env.ExternalScoped(t, "contacts:read", "contacts:write", "contacts:erase")
	before := env.DB.Contact.GetX(ctx, int64(foreign))

	get, err := c.ContactsGet(ctx, externalapi.ContactsGetParams{ID: foreign})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsGetNotFound{}, get)
	upd, err := c.ContactsUpdate(ctx, &externalapi.UpdateContactInput{FirstName: externalapi.NewOptNilString("Hijack")}, externalapi.ContactsUpdateParams{ID: foreign})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsUpdateNotFound{}, upd)
	del, err := c.ContactsDelete(ctx, externalapi.ContactsDeleteParams{ID: foreign})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsDeleteNotFound{}, del)

	still, err := env.DB.Contact.Get(ctx, int64(foreign))
	require.NoError(t, err)
	assert.Equal(t, before.FirstName, still.FirstName, "the foreign contact is untouched")

	missing, err := c.ContactsUpdate(ctx, &externalapi.UpdateContactInput{}, externalapi.ContactsUpdateParams{ID: 999999})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsUpdateNotFound{}, missing)
	gone, err := c.ContactsDelete(ctx, externalapi.ContactsDeleteParams{ID: 999999})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsDeleteNotFound{}, gone)
}

func TestExternalContactsListRequiresReadScopeAndPaginates(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	denied, err := env.ExternalScoped(t, "contacts:write").ContactsList(ctx, externalapi.ContactsListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsListForbidden{}, denied)

	c := env.ExternalScoped(t, "contacts:read")
	first, err := c.ContactsList(ctx, externalapi.ContactsListParams{PageSize: externalapi.NewOptInt32(1)})
	require.NoError(t, err)
	p1 := first.(*externalapi.ContactsListOK)
	require.Len(t, p1.Items, 1)
	assert.Greater(t, p1.TotalItems, int32(1))
	assert.Equal(t, p1.TotalItems, p1.TotalPages, "one item per page")

	second, err := c.ContactsList(ctx, externalapi.ContactsListParams{PageSize: externalapi.NewOptInt32(1), Page: externalapi.NewOptInt32(2)})
	require.NoError(t, err)
	p2 := second.(*externalapi.ContactsListOK)
	require.Len(t, p2.Items, 1)
	assert.NotEqual(t, p1.Items[0].ID, p2.Items[0].ID)
}

func TestExternalContactsBatchUpsertCarriesCustomFields(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "contacts:write")

	res, err := c.ContactsBatchUpsert(ctx, &externalapi.UpsertContactsInput{Contacts: []externalapi.UpsertContactInput{{
		Email: externalapi.NewOptNilEmailAddress("batch.cf@example.com"),
		CustomFields: externalapi.NewOptNilUpsertContactInputCustomFields(externalapi.UpsertContactInputCustomFields{
			"tier": jx.Raw(`"gold"`),
		}),
	}}})
	require.NoError(t, err)
	out := res.(*externalapi.UpsertContactsResult)
	require.Len(t, out.Results, 1)
	assert.Equal(t, externalapi.ContactBatchStatusCreated, out.Results[0].Status)
	stored, err := env.DB.Contact.Get(ctx, int64(out.Results[0].ContactId.Value))
	require.NoError(t, err)
	assert.Equal(t, "gold", stored.CustomFields["tier"])
}

// A batch item's failure is worded for the caller: a conflict and a missing alias key say
// so in the module's own words.
func TestABatchItemFailureIsWordedForTheCaller(t *testing.T) {
	env := testhelper.Setup(t)
	taken := env.DB.Contact.GetX(t.Context(), fixtures.ContactExportSubjectID).Phone
	require.NotNil(t, taken)

	res, err := env.ExternalScoped(t, "contacts:write").ContactsBatchUpsert(t.Context(), &externalapi.UpsertContactsInput{
		Contacts: []externalapi.UpsertContactInput{
			{Email: externalapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail), Phone: externalapi.NewOptNilString(*taken)},
			{FirstName: externalapi.NewOptNilString("Nobody")},
		},
	})
	require.NoError(t, err)
	out, ok := res.(*externalapi.UpsertContactsResult)
	require.Truef(t, ok, "got %T", res)
	require.Len(t, out.Results, 2)

	assert.Equal(t, contacts.ErrPhoneTaken.Message(), out.Results[0].Error.Or(""))
	assert.Equal(t, contacts.ErrIdentityRequired.Message(), out.Results[1].Error.Or(""))
}

// errStorageDown is a storage failure whose words name an address the caller must never see.
var errStorageDown = errors.New("pq: connection refused at 10.0.0.3")

// isContact is the condition of a hook that touches only Contact writes.
func isContact(_ context.Context, m ent.Mutation) bool { return m.Type() == ent.TypeContact }

// A batch item that fails in storage is an internal error to the caller: what the database
// said never leaks.
func TestABatchItemStorageFailureHidesTheCause(t *testing.T) {
	env := testhelper.Setup(t)
	env.Bus.Use(hook.If(hook.FixedError(errStorageDown), isContact))

	res, err := env.ExternalScoped(t, "contacts:write").ContactsBatchUpsert(t.Context(), &externalapi.UpsertContactsInput{
		Contacts: []externalapi.UpsertContactInput{{Email: externalapi.NewOptNilEmailAddress("batch.down@example.com")}},
	})
	require.NoError(t, err)
	out, ok := res.(*externalapi.UpsertContactsResult)
	require.Truef(t, ok, "got %T", res)
	require.Len(t, out.Results, 1)

	assert.Equal(t, externalapi.ContactBatchStatusFailed, out.Results[0].Status)
	assert.Equal(t, "internal error", out.Results[0].Error.Or(""))
}

// JSON Merge Patch on update, as on /site: an explicit null clears an optional field
// and an absent key leaves it unchanged. The contract declares every field nullable.
func TestExternalContactsUpdateNullClearsAbsentKeeps(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "contacts:read", "contacts:write")

	var null externalapi.OptNilString
	null.SetToNull()
	res, err := c.ContactsUpdate(ctx, &externalapi.UpdateContactInput{FirstName: null},
		externalapi.ContactsUpdateParams{ID: entityIDString(fixtures.ContactAliceID)})
	require.NoError(t, err)
	got, ok := res.(*externalapi.ContactResource)
	require.Truef(t, ok, "got %T", res)
	assert.Empty(t, got.FirstName.Value, "first name cleared by explicit null")

	stored, err := env.DB.Contact.Get(ctx, fixtures.ContactAliceID)
	require.NoError(t, err)
	assert.Nil(t, stored.FirstName, "the clear is persisted")
	assert.Equal(t, "Smith", *stored.LastName, "an absent last name is kept")
	assert.Equal(t, fixtures.ContactAliceEmail, *stored.Email, "an absent email is kept")
}
