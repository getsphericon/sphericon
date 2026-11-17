package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
)

type Segment struct {
	ent.Schema
}

func (Segment) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "segments", Checks: map[string]string{
			"segments_condition_check": conditionCheck("condition"),
		}},
		Audited{Action: "segment", NameField: NameColumn},
	}
}

func (Segment) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "segments"}, ArchiveMixin{}, RevisionMixin{}}
}

func (Segment) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.String(NameColumn).
			NotEmpty(),
		// The Condition (conditiontree.Group), jsonb (ADR 0169); the empty group, with no tests,
		// matches every Contact.
		field.JSON("condition", conditiontree.Group{}).
			Default(conditiontree.Empty()).
			Annotations(entsql.Default(emptyConditionSQL)),
	}
}
