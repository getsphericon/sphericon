package contacts_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestAnUpsertClearsAnAttributeItIsGivenNullFor(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)

	res, err := m.Upsert(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceAPI, contacts.UpsertInput{
		Email:    lo.ToPtr(fixtures.ContactAliceEmail),
		LastName: patch.Clear[string](),
	})
	require.NoError(t, err)

	assert.Nil(t, res.Contact.LastName, "null clears")
	assert.Equal(t, "Alice", lo.FromPtr(res.Contact.FirstName), "an absent attribute keeps")
}

func TestAnUpsertNeverOverwritesAnAliasKey(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)
	ctx := context.Background()
	stark := env.DB.Scoped(fixtures.StarkID)

	res, err := m.Upsert(ctx, stark, contact.SourceAPI, contacts.UpsertInput{Email: new(fixtures.ContactStarkAliasedEmail), Phone: new("+14155550114")})
	require.NoError(t, err)

	assert.EqualValues(t, fixtures.ContactStarkAliasedID, res.Contact.ID)
	assert.Equal(t, fixtures.ContactStarkAliasedPhone, lo.FromPtr(res.Contact.Phone), "an alias key fills only a Contact that has none")
}

func TestTheBrowsersGuessesFillOnlyAnEmptyAttributeAndLoseToATrait(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)

	created, err := m.Upsert(ctx, acme, contact.SourceTracker, contacts.UpsertInput{
		Email: new("guessed@example.com"), GuessedTimeZone: new("Europe/Berlin"), GuessedLocale: new("de-de"),
	})
	require.NoError(t, err)
	assert.Equal(t, "Europe/Berlin", lo.FromPtr(created.Contact.TimeZone), "a guess fills an empty time zone")
	assert.Equal(t, "de-DE", lo.FromPtr(created.Contact.Locale), "and an empty locale, canonical")

	moved, err := m.Upsert(ctx, acme, contact.SourceTracker, contacts.UpsertInput{
		Email: new("guessed@example.com"), GuessedTimeZone: new("Asia/Tokyo"), GuessedLocale: new("ja"),
	})
	require.NoError(t, err)
	assert.Equal(t, "Europe/Berlin", lo.FromPtr(moved.Contact.TimeZone), "a guess never replaces a stored value")
	assert.Equal(t, "de-DE", lo.FromPtr(moved.Contact.Locale))

	asserted, err := m.Upsert(ctx, acme, contact.SourceTracker, contacts.UpsertInput{
		Email: new("guessed@example.com"), TimeZone: patch.Set("Asia/Tokyo"),
		GuessedTimeZone: new("America/New_York"),
	})
	require.NoError(t, err)
	assert.Equal(t, "Asia/Tokyo", lo.FromPtr(asserted.Contact.TimeZone), "a trait wins over the guess")
}

func TestAnInvalidGuessIsIgnored(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)

	res, err := m.Upsert(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceTracker, contacts.UpsertInput{
		Email: new("bad.guess@example.com"), GuessedTimeZone: new("Mars/Olympus"), GuessedLocale: new("not a tag"),
	})
	require.NoError(t, err)
	assert.Nil(t, res.Contact.TimeZone)
	assert.Nil(t, res.Contact.Locale)
}

func TestNoCustomFieldTakesTheKeyOfACoreAttribute(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)

	_, err := m.Create(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, contacts.CreateInput{
		Email: new("reserved@example.com"), CustomFields: map[string]any{"country": "DE"}})
	assert.ErrorIs(t, err, contacts.ErrReservedKey)
}
