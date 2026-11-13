// Package givenupnotbool is a schema internal/entgen's tests load: a field a Discard gives up that is not a bool.
package givenupnotbool

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
	return []ent.Mixin{schema.TimeMixin{}, schema.WorkspaceMixin{Ref: "things"}, schema.DiscardMixin{}}
}

func (Thing) Fields() []ent.Field {
	return []ent.Field{
		field.String("label").Annotations(schema.GivenUpOnDiscard{}),
	}
}
