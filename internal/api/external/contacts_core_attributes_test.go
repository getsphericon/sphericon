package external_test

import (
	"context"
	"testing"
	"time"

	"github.com/go-faster/jx"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// ADR 0072: the upsert overwrites a core attribute it is given and clears one it is given null for.
func TestExternalUpsertOverwritesCoreAttributes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "contacts:write")

	res, err := c.ContactsBatchUpsert(ctx, &externalapi.UpsertContactsInput{Contacts: []externalapi.UpsertContactInput{{
		Email:     externalapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail),
		FirstName: externalapi.NewOptNilString("Alicia"),
		LastName:  externalapi.OptNilString{Set: true, Null: true},
		Locale:    externalapi.NewOptNilLocaleTag("pt-br"),
		Birthday:  externalapi.NewOptNilDate(time.Date(1990, time.May, 1, 0, 0, 0, 0, time.UTC)),
		City:      externalapi.NewOptNilString("Lisbon"),
		Avatar:    externalapi.NewOptNilImageUrl("https://example.com/alice.png"),
	}}})
	require.NoError(t, err)
	require.IsType(t, &externalapi.UpsertContactsResult{}, res)

	stored, err := env.DB.Contact.Get(ctx, fixtures.ContactAliceID)
	require.NoError(t, err)
	assert.Equal(t, "Alicia", lo.FromPtr(stored.FirstName), "a given attribute replaces the stored one")
	assert.Nil(t, stored.LastName, "null clears")
	assert.Equal(t, "pt-BR", lo.FromPtr(stored.Locale), "a locale is stored canonical")
	assert.Equal(t, "1990-05-01", stored.Birthday.Format(time.DateOnly))
	assert.Equal(t, "Lisbon", lo.FromPtr(stored.City))
	assert.Equal(t, "https://example.com/alice.png", lo.FromPtr(stored.Avatar))
}

func TestExternalContactRefusesACustomFieldNamedAfterACoreAttribute(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")

	res, err := c.ContactsCreate(context.Background(), &externalapi.CreateContactInput{
		Email:        externalapi.NewOptNilEmailAddress("reserved.key@example.com"),
		CustomFields: externalapi.NewOptNilJsonObject(externalapi.JsonObject{"city": jx.Raw(`"Berlin"`)}),
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsCreateUnprocessableEntity{}, res)
}

func TestExternalContactRefusesAPhotoThatIsNotHTTPS(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")

	res, err := c.ContactsCreate(context.Background(), &externalapi.CreateContactInput{
		Email:  externalapi.NewOptNilEmailAddress("plain.http@example.com"),
		Avatar: externalapi.NewOptNilImageUrl("http://example.com/a.png"),
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsCreateBadRequest{}, res, "the contract's pattern refuses it")
}
