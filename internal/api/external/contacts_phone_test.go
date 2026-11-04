package external_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// ADR 0075: a phone is stored in E.164 whatever format a door is given it in; a number without a
// country code is read with the Contact's country.
func TestExternalContactsCreateStoresThePhoneInE164(t *testing.T) {
	cases := []struct {
		name, phone string
		country     externalapi.OptNilCountryCode
		want        string
	}{
		{"international with punctuation", "+1 (415) 555-0100", externalapi.OptNilCountryCode{}, "+14155550100"},
		{"the 00 international prefix", "0049 30 1234567", externalapi.OptNilCountryCode{}, "+49301234567"},
		{"local, read with the country", "030 1234567", externalapi.NewOptNilCountryCode("DE"), "+49301234567"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			c := env.ExternalScoped(t, "contacts:write")

			res, err := c.ContactsCreate(context.Background(), &externalapi.CreateContactInput{
				Phone:   externalapi.NewOptNilString(tc.phone),
				Country: tc.country,
			})
			require.NoError(t, err)
			got, ok := res.(*externalapi.ContactResource)
			require.Truef(t, ok, "got %T", res)
			assert.Equal(t, tc.want, got.Phone.Value)
		})
	}
}

func TestExternalContactsUpdateReadsALocalPhoneWithTheStoredCountry(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "contacts:write")
	id := entityIDString(fixtures.ContactAliceID)
	_, err := c.ContactsUpdate(ctx, &externalapi.UpdateContactInput{Country: externalapi.NewOptNilCountryCode("US")},
		externalapi.ContactsUpdateParams{ID: id})
	require.NoError(t, err)

	res, err := c.ContactsUpdate(ctx, &externalapi.UpdateContactInput{Phone: externalapi.NewOptNilString("(415) 555-0101")},
		externalapi.ContactsUpdateParams{ID: id})
	require.NoError(t, err)
	got, ok := res.(*externalapi.ContactResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, "+14155550101", got.Phone.Value)
}

func TestExternalContactsUpdateReadsALocalPhoneWithTheCountryItSets(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")

	res, err := c.ContactsUpdate(context.Background(), &externalapi.UpdateContactInput{
		Phone:   externalapi.NewOptNilString("8 999 123-45-67"),
		Country: externalapi.NewOptNilCountryCode("RU"),
	}, externalapi.ContactsUpdateParams{ID: entityIDString(fixtures.ContactAliceID)})
	require.NoError(t, err)
	got, ok := res.(*externalapi.ContactResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, "+79991234567", got.Phone.Value)
}

func TestExternalContactsRefuseAPhoneThatIsNoNumber(t *testing.T) {
	cases := []struct{ name, phone string }{
		{"not a number", "call me maybe"},
		{"local, and the Contact has no country", "415 555 0100"},
		{"with an extension", "+1 415 555 0100 ext. 12"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			c := env.ExternalScoped(t, "contacts:write")

			created, err := c.ContactsCreate(context.Background(), &externalapi.CreateContactInput{
				Email: externalapi.NewOptNilEmailAddress("bad.phone@example.com"),
				Phone: externalapi.NewOptNilString(tc.phone),
			})
			require.NoError(t, err)
			refused, ok := created.(*externalapi.ContactsCreateUnprocessableEntity)
			require.Truef(t, ok, "got %T", created)
			assert.Contains(t, refused.Errors.Value, "phone")

			updated, err := c.ContactsUpdate(context.Background(), &externalapi.UpdateContactInput{
				Phone: externalapi.NewOptNilString(tc.phone),
			}, externalapi.ContactsUpdateParams{ID: entityIDString(fixtures.ContactAliceID)})
			require.NoError(t, err)
			assert.IsType(t, &externalapi.ContactsUpdateUnprocessableEntity{}, updated)
		})
	}
}

// An import reports the row whose phone is no number and still writes the others.
func TestExternalContactsBatchUpsertReportsARowWithABadPhone(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "contacts:write")

	res, err := c.ContactsBatchUpsert(ctx, &externalapi.UpsertContactsInput{Contacts: []externalapi.UpsertContactInput{
		{Email: externalapi.NewOptNilEmailAddress("good.row@example.com"), Phone: externalapi.NewOptNilString("+1 415 555 0102")},
		{Email: externalapi.NewOptNilEmailAddress("bad.row@example.com"), Phone: externalapi.NewOptNilString("555-0103")},
		{Email: externalapi.NewOptNilEmailAddress("local.row@example.com"), Phone: externalapi.NewOptNilString("020 7946 0958"),
			Country: externalapi.NewOptNilCountryCode("GB")},
	}})
	require.NoError(t, err)
	got, ok := res.(*externalapi.UpsertContactsResult)
	require.Truef(t, ok, "got %T", res)
	require.Len(t, got.Results, 3)

	assert.Equal(t, externalapi.ContactBatchStatusCreated, got.Results[0].Status)
	assert.Equal(t, externalapi.ContactBatchStatusFailed, got.Results[1].Status)
	assert.Contains(t, got.Results[1].Error.Value, "phone")
	assert.Equal(t, externalapi.ContactBatchStatusCreated, got.Results[2].Status)

	good, err := env.DB.Contact.Get(ctx, int64(got.Results[0].ContactId.Value))
	require.NoError(t, err)
	assert.Equal(t, "+14155550102", lo.FromPtr(good.Phone))
	local, err := env.DB.Contact.Get(ctx, int64(got.Results[2].ContactId.Value))
	require.NoError(t, err)
	assert.Equal(t, "+442079460958", lo.FromPtr(local.Phone))
}

// One number in two formats is one Contact: the upsert resolves the stored E.164 phone.
func TestExternalContactsBatchUpsertResolvesAPhoneInAnotherFormat(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "contacts:write")
	created, err := c.ContactsCreate(ctx, &externalapi.CreateContactInput{Phone: externalapi.NewOptNilString("+14155550104")})
	require.NoError(t, err)
	first, ok := created.(*externalapi.ContactResource)
	require.Truef(t, ok, "got %T", created)

	res, err := c.ContactsBatchUpsert(ctx, &externalapi.UpsertContactsInput{Contacts: []externalapi.UpsertContactInput{{
		Phone:   externalapi.NewOptNilString("(415) 555-0104"),
		Country: externalapi.NewOptNilCountryCode("US"),
	}}})
	require.NoError(t, err)
	got, ok := res.(*externalapi.UpsertContactsResult)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, externalapi.ContactBatchStatusUpdated, got.Results[0].Status)
	assert.Equal(t, first.ID, got.Results[0].ContactId.Value)
}
