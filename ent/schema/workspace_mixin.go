package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// WorkspaceMixin scopes an entity to a Workspace (ADR 0017): the workspace_id
// column and the required workspace edge. Ref names the reverse edge on
// Workspace, which stays hand-written there. Schema-specific indexes over
// workspace_id stay in their schemas.
type WorkspaceMixin struct {
	mixin.Schema

	Ref string
}

func (WorkspaceMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("workspace_id"),
	}
}

func (m WorkspaceMixin) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("workspace", Workspace.Type).
			Ref(m.Ref).
			Field("workspace_id").
			Required().
			Unique(),
	}
}
