// Package givenupwithoutdiscard is a schema internal/entgen's tests load: a field a Discard gives up on an entity that is never discarded.
package givenupwithoutdiscard

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"

	"github.com/getsphericon/sphericon/ent/schema"
)

// Workspace is the tenant root the WorkspaceMixin's edge names.
type Workspace struct{ ent.Schema }

func (Workspace) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("things", Thing.Type),
	}
}

type Thing struct{ ent.Schema }

func (Thing) Mixin() []ent.Mixin {
	return []ent.Mixin{schema.TimeMixin{}, schema.WorkspaceMixin{Ref: "things"}, schema.ArchiveMixin{}}
}

func (Thing) Fields() []ent.Field {
	return []ent.Field{
		field.Bool("is_default").Annotations(schema.Attribute("a setting its owner edits"), schema.GivenUpOnDiscard{}),
	}
}
