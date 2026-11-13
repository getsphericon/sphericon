// Package workspaceoutsidemixin is a schema internal/entgen's tests load: Workspace columns declared outside WorkspaceMixin, marked and not.
package workspaceoutsidemixin

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
		edge.To("journals", Journal.Type),
	}
}

// Journal declares the Workspace column by hand.
type Journal struct{ ent.Schema }

func (Journal) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("workspace_id"),
	}
}

func (Journal) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("workspace", Workspace.Type).Ref("journals").Field("workspace_id").Required().Unique(),
	}
}

// Trace marks its Workspace column without a reason.
type Trace struct{ ent.Schema }

func (Trace) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("workspace_id").Annotations(
			schema.NotWorkspaceOwned{},
			schema.ScopedRef{Unchecked: true, Reason: "a tombstone of the Workspace"},
		),
	}
}

// Tombstone marks its Workspace column with a reason.
type Tombstone struct{ ent.Schema }

func (Tombstone) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("workspace_id").Annotations(
			schema.NotWorkspaceOwned{Reason: "the entry outlives its Workspace"},
			schema.ScopedRef{Unchecked: true, Reason: "a tombstone of the Workspace"},
		),
	}
}
