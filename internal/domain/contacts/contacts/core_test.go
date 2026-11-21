package contacts_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
)

func TestATraitOfACoreAttributeWritesTheAttributeNotACustomField(t *testing.T) {
	core, fields := contacts.FromTraits(map[string]any{
		fieldname.FirstName: " Ada ",
		fieldname.LastName:  "Lovelace",
		fieldname.TimeZone:  "Europe/London",
		fieldname.Country:   "gb",
		fieldname.Locale:    "en-gb",
		fieldname.Birthday:  "1815-12-10",
		fieldname.City:      "London",
		fieldname.Avatar:    "https://example.com/ada.png",
		"plan":              "pro",
	})

	assert.Equal(t, contacts.Core{
		FirstName: patch.Set("Ada"),
		LastName:  patch.Set("Lovelace"),
		TimeZone:  patch.Set("Europe/London"),
		Country:   patch.Set("GB"),
		Locale:    patch.Set("en-GB"),
		Birthday:  patch.Set(time.Date(1815, time.December, 10, 0, 0, 0, 0, time.UTC)),
		City:      patch.Set("London"),
		Avatar:    patch.Set("https://example.com/ada.png"),
	}, core)
	assert.Equal(t, map[string]any{"plan": "pro"}, fields, "only a trait no core attribute names is a Custom field")
}

func TestANullTraitClearsItsAttribute(t *testing.T) {
	core, fields := contacts.FromTraits(map[string]any{fieldname.Country: nil, fieldname.Birthday: nil})

	assert.True(t, core.Country.IsClear())
	assert.True(t, core.Birthday.IsClear())
	assert.Empty(t, fields)
}

func TestAnInvalidValueOfACoreAttributeIsDropped(t *testing.T) {
	core, fields := contacts.FromTraits(map[string]any{
		fieldname.TimeZone:  "Mars/Olympus",
		fieldname.Country:   "USA",
		fieldname.Locale:    "not a tag",
		fieldname.Birthday:  "10/12/1815",
		fieldname.Avatar:    "http://example.com/ada.png",
		fieldname.FirstName: 42.0,
		fieldname.City:      "   ",
	})

	assert.Equal(t, contacts.Core{}, core, "every attribute keeps its stored value")
	assert.Empty(t, fields, "and none becomes a Custom field")
}

func TestATraitOfAnAliasKeyIsDropped(t *testing.T) {
	core, fields := contacts.FromTraits(map[string]any{"email": "a@example.com", "phone": "+1", "subjectId": "u-1"})

	assert.Equal(t, contacts.Core{}, core)
	assert.Empty(t, fields, "Identify carries its alias keys itself")
}

func TestASnakeCaseTraitIsAnOrdinaryCustomField(t *testing.T) {
	core, fields := contacts.FromTraits(map[string]any{"first_name": "Ada"})

	assert.Equal(t, contacts.Core{}, core)
	assert.Equal(t, map[string]any{"first_name": "Ada"}, fields)
}
