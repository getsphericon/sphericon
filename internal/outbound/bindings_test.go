package outbound_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/outbound"
)

// A Contact's city, country and locale are merge tags; the country reads as its English name, and
// a Custom field of the same name never shadows a core one.
func TestACoreAttributeIsAMergeTagAndTheCountryReadsByName(t *testing.T) {
	c := &ent.Contact{
		City: new("Berlin"), Country: new("DE"), Locale: new("de-DE"),
		CustomFields: map[string]any{"city": "forged"},
	}

	b := outbound.Bindings(c, nil)

	assert.Equal(t, "Berlin", b["city"])
	assert.Equal(t, "Germany", b["country"])
	assert.Equal(t, "de-DE", b["locale"])
}

func TestAContactWithoutTheAttributesBindsThemEmpty(t *testing.T) {
	b := outbound.Bindings(&ent.Contact{}, nil)

	assert.Empty(t, b["city"])
	assert.Empty(t, b["country"])
	assert.Empty(t, b["locale"])
}
