// Package auditalways is a schema internal/entgen's tests load: an Audited entity whose Always names an unknown field.
package auditalways

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
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
		field.String("name"),
	}
}

func (Thing) Annotations() []entschema.Annotation {
	return []entschema.Annotation{schema.Audited{Action: "thing", Always: []string{"kind"}}}
}
