// Package refunknownentity is a schema internal/entgen's tests load: a ScopedRef that names an unknown entity.
package refunknownentity

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
	return []ent.Mixin{schema.TimeMixin{}, schema.WorkspaceMixin{Ref: "things"}}
}

func (Thing) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("ghost_id").Annotations(schema.ScopedRef{Entity: "Ghost"}),
	}
}
