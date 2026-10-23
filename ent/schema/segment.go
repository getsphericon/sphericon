package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Segment struct {
	ent.Schema
}

func (Segment) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "segments"},
		Audited{Action: "segment", NameField: nameField},
	}
}

func (Segment) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "segments"}, DiscardMixin{}}
}

func (Segment) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.String(nameField).
			NotEmpty(),
		field.String("condition").
			Optional().
			Nillable(),
	}
}

func (Segment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id"),
	}
}
