// Package archivedanddiscarded is a schema internal/entgen's tests load: an entity both archived and discarded.
package archivedanddiscarded

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"

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
	return []ent.Mixin{schema.TimeMixin{}, schema.WorkspaceMixin{Ref: "things"}, schema.ArchiveMixin{}, schema.DiscardMixin{}}
}
