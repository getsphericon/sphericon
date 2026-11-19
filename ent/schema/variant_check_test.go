package schema_test

import (
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entschema "github.com/getsphericon/sphericon/ent/schema"
)

// A table that keeps several kinds of row checks which columns each kind owns with one arm per
// value of its discriminator (variantCheck). The arms are ORed, so a value without one fails the
// CHECK on every row of it: a value added to the enum without its arm would be refused by the
// database at its first write.
func TestEveryVariantCheckHasAnArmPerValue(t *testing.T) {
	type table interface {
		Fields() []ent.Field
		Annotations() []schema.Annotation
	}
	for _, c := range []struct {
		table  table
		check  string
		column string
	}{
		{entschema.Binding{}, "bindings_destination_check", "channel"},
		{entschema.Broadcast{}, "broadcasts_content_check", "channel"},
		{entschema.OutboundMessage{}, "outbound_messages_provenance_check", "kind"},
		{entschema.OutboundMessage{}, "outbound_messages_outcome_check", "status"},
	} {
		t.Run(c.check, func(t *testing.T) {
			expr := checkOf(t, c.table.Annotations(), c.check)
			values := enumValues(t, c.table.Fields(), c.column)
			require.NotEmpty(t, values)
			for _, v := range values {
				assert.Contains(t, expr, "("+c.column+" = '"+v+"'", "%s has no arm for %q", c.check, v)
			}
		})
	}
}

// checkOf is the expression of the CHECK name among a table's annotations.
func checkOf(t *testing.T, annotations []schema.Annotation, name string) string {
	t.Helper()
	for _, a := range annotations {
		var checks map[string]string
		switch sql := a.(type) {
		case entsql.Annotation:
			checks = sql.Checks
		case *entsql.Annotation:
			checks = sql.Checks
		}
		if expr, ok := checks[name]; ok {
			return expr
		}
	}
	require.FailNow(t, "no such check", name)
	return ""
}

// enumValues are the values of the enum field column among fields.
func enumValues(t *testing.T, fields []ent.Field, column string) []string {
	t.Helper()
	var values []string
	for _, f := range fields {
		if d := f.Descriptor(); d.Name == column {
			for _, e := range d.Enums {
				values = append(values, e.V)
			}
		}
	}
	return values
}
