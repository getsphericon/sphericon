package contacts_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The module reads a country as an ISO 3166-1 alpha-2 code on every write, whatever door it comes
// through: a lower-case or padded code is stored upper case, anything else is refused on the
// country field instead of reaching the database's check.
func TestEveryWriteReadsTheCountryAsAnISOCode(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	c, err := m.Create(ctx, s, contact.SourceManual, contacts.CreateInput{Email: new("country@example.com"), Country: new(" de ")})
	require.NoError(t, err)
	assert.Equal(t, "DE", *c.Country)

	for name, write := range map[string]func() error{
		"create": func() error {
			_, err := m.Create(ctx, s, contact.SourceManual, contacts.CreateInput{Email: new("usa@example.com"), Country: new("usa")})
			return err
		},
		"update": func() error {
			_, err := m.Update(ctx, s, fixtures.ContactAliceID, contacts.UpdateInput{Revision: testhelper.ContactRevisionIn(ctx, t, s, fixtures.ContactAliceID), Country: patch.Set("U5")})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := write()
			var refusal *validation.RefusalError
			require.ErrorAs(t, err, &refusal)
			assert.Equal(t, "country", refusal.Field())
		})
	}
}

// The database refuses a phone or a country a write did not normalise, whatever code writes it
// (contacts_phone_format, contacts_country_format).
func TestTheDatabaseRefusesAnUnnormalisedPhoneOrCountry(t *testing.T) {
	for name, change := range map[string]func(c *ent.ContactScopedCreate){
		"a phone with punctuation": func(c *ent.ContactScopedCreate) { c.SetPhone("+1 (415) 555-0100") },
		"a local phone":            func(c *ent.ContactScopedCreate) { c.SetPhone("4155550100") },
		"a lower-case country":     func(c *ent.ContactScopedCreate) { c.SetCountry("de") },
	} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			err := env.Bus.WithinTx(t.Context(), func(tx *ent.Client, _ events.Publisher) error {
				// test writes: the refused insert is the operation under test.
				create := tx.Scoped(fixtures.AcmeID).Contact().Create().SetEmail("raw@example.com").SetSource(contact.SourceManual)
				change(create)
				return create.Exec(t.Context())
			})
			require.Error(t, err)
			assert.True(t, ent.IsConstraintError(err), "a constraint refuses it: %v", err)
		})
	}
}
