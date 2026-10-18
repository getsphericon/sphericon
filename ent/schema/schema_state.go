package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// SchemaState is the one row that records the database's Major and the oldest release that
// still runs on it (ADR 0033): the guard in internal/db compares a starting binary with it.
// The first migration inserts the row (id 1); expand-only migrations never touch it and a
// destructive migration writes both fields. It belongs to the instance, not a Workspace.
type SchemaState struct {
	ent.Schema
}

func (SchemaState) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "schema_state"},
	}
}

func (SchemaState) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// Major is the schema's Major as a counter: 1 for the first, plus one for each
		// destructive migration. Releases map to it through internal/db's table of Majors.
		field.Int("major"),
		// Floor is the oldest release (a semver tag such as v0.4.7) that still runs on
		// this schema: the last release of the previous Major.
		field.String("floor"),
	}
}
