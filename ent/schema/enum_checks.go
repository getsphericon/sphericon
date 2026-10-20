package schema

import (
	"fmt"
	"strings"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
)

// EnumChecks is the CHECK constraint for every enum field of a schema, so the database
// refuses a value the model does not list: ent stores an enum as a plain varchar. The
// values come from the fields themselves, so a new value cannot drift from its
// constraint. Pass the schema's own Fields(); the constraint is named
// <table>_<column>_check.
func EnumChecks(table string, fields []ent.Field) map[string]string {
	checks := map[string]string{}
	for _, f := range fields {
		d := f.Descriptor()
		if len(d.Enums) == 0 {
			continue
		}
		values := make([]string, len(d.Enums))
		for i, e := range d.Enums {
			values[i] = "'" + strings.ReplaceAll(e.V, "'", "''") + "'"
		}
		column := d.StorageKey
		if column == "" {
			column = d.Name
		}
		checks[fmt.Sprintf("%s_%s_check", table, column)] = fmt.Sprintf("%s IN (%s)", column, strings.Join(values, ", "))
	}
	return checks
}

// TableWithEnumChecks is the table annotation of a schema whose enum fields are checked.
func TableWithEnumChecks(table string, fields []ent.Field) entsql.Annotation {
	return entsql.Annotation{Table: table, Checks: EnumChecks(table, fields)}
}
