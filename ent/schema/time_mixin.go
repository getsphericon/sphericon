package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// TimeMixin gives every entity created_at and updated_at (ADR 0017), including
// append-only ones, for uniformity. The database default lets the columns be
// added to tables that already hold rows.
type TimeMixin struct {
	mixin.Schema
}

func (TimeMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Time("created_at").
			Annotations(Attribute(reasonHistory)).
			Default(time.Now).
			Immutable().
			Annotations(entsql.Default("CURRENT_TIMESTAMP")),
		field.Time("updated_at").
			Annotations(Attribute(reasonHistory)).
			Default(time.Now).
			UpdateDefault(time.Now).
			Annotations(entsql.Default("CURRENT_TIMESTAMP")),
	}
}
