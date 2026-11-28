package contacts_test

import (
	"encoding/json"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
)

// A Custom field's type reads a value of its own JSON type only (ADR 0125): no coercion, so "42"
// is no number and "true" no boolean, and an object, a list and null are read by no type.
func TestACustomFieldTypeReadsOnlyAValueOfItsOwnJSONType(t *testing.T) {
	values := map[string]any{
		"number":        42.0,
		"json number":   json.Number("42"),
		"boolean":       true,
		"string":        "pro",
		"numeric text":  "42",
		"ISO date":      "2026-01-15",
		"ISO date-time": "2026-01-15T10:00:00Z",
		"object":        map[string]any{"a": 1.0},
		"list":          []any{1.0},
		"null":          nil,
	}
	for _, c := range []struct {
		fieldType customfield.Type
		reads     []string
	}{
		{customfield.TypeNumber, []string{"number", "json number"}},
		{customfield.TypeBool, []string{"boolean"}},
		{customfield.TypeString, []string{"string", "numeric text", "ISO date", "ISO date-time"}},
		{customfield.TypeDatetime, []string{"ISO date", "ISO date-time"}},
	} {
		t.Run(string(c.fieldType), func(t *testing.T) {
			read := lo.Filter(lo.Keys(values), func(name string, _ int) bool { return contacts.TypeReads(c.fieldType, values[name]) })

			assert.ElementsMatch(t, c.reads, read)
		})
	}
}
