// Package enumwithoutcheck is a schema internal/entgen's tests load: enum fields without the CHECK
// constraint named after their table and column, and one with it.
package enumwithoutcheck

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/field"

	"github.com/getsphericon/sphericon/ent/schema"
)

// Unchecked has an enum and no CHECK constraint for it.
type Unchecked struct{ ent.Schema }

func (Unchecked) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("color").Values("red", "blue"),
	}
}

// Misnamed checks its enum under the name of another table, as a mixin given the wrong table would.
type Misnamed struct{ ent.Schema }

func (Misnamed) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("shade").Values("light", "dark"),
	}
}

func (Misnamed) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{Checks: schema.EnumChecks("others", Misnamed{}.Fields())}}
}

// Checked checks its enum under its own table's name.
type Checked struct{ ent.Schema }

func (Checked) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("tone").Values("warm", "cold"),
	}
}

func (Checked) Annotations() []entschema.Annotation {
	return []entschema.Annotation{schema.TableWithEnumChecks("checkeds", Checked{}.Fields())}
}
