// Package statewithoutform is a schema internal/entgen's tests load: state-shaped fields that declare no form.
package statewithoutform

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
		field.Bool("enabled"),
		field.Time("seen_at"),
		field.Enum("status").Values("on", "off"),
		field.String("note"),
	}
}
