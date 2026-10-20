package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CustomField is the workspace-scoped, typed, named definition of a Contact
// attribute beyond the core fields (email / phone / subject_id / name). It is the
// one attribute concept — there is no schemaless "trait". An unknown key arriving
// from Identify is auto-created here with an inferred type (declared-by-use), but is
// a first-class, renameable definition from the first sight. Per-contact values live
// in Contact.custom_fields keyed by this field's `key`.
type CustomField struct {
	ent.Schema
}

func (CustomField) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "custom_fields"},
		Audited{Action: "custom_field", NameField: "name"},
	}
}

func (CustomField) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "custom_fields"}}
}

func (CustomField) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// Machine key — how values are addressed in Contact.custom_fields and segment
		// rules. Stable; the display name is renameable independently.
		field.String("key").
			NotEmpty(),
		field.String("name").
			NotEmpty(),
		// Inferred on first sight; later values of a conflicting type widen to string
		// (coercion is handled at ingest, not constrained here).
		field.Enum("type").
			Values("string", "number", "bool", "datetime").
			Default("string"),
	}
}

func (CustomField) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "key").Unique().StorageKey("custom_fields_workspace_id_key"),
	}
}
