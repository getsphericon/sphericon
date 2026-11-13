// Package markerwithoutmixin is a schema internal/entgen's tests load: entities that declare a kind
// by its marker alone, without the mixin that brings the kind's column.
package markerwithoutmixin

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"

	"github.com/getsphericon/sphericon/ent/schema"
)

// Owned declares the Workspace-owned kind and the revision without their columns.
type Owned struct{ ent.Schema }

func (Owned) Mixin() []ent.Mixin { return []ent.Mixin{schema.TimeMixin{}} }

func (Owned) Annotations() []entschema.Annotation {
	return []entschema.Annotation{schema.WorkspaceOwned{}, schema.Revised{}}
}

// Aside declares the archivable kind without the archived_at column.
type Aside struct{ ent.Schema }

func (Aside) Mixin() []ent.Mixin { return []ent.Mixin{schema.TimeMixin{}} }

func (Aside) Annotations() []entschema.Annotation {
	return []entschema.Annotation{schema.Archivable{}}
}

// Gone declares the discardable kind without the discarded_at column.
type Gone struct{ ent.Schema }

func (Gone) Mixin() []ent.Mixin { return []ent.Mixin{schema.TimeMixin{}} }

func (Gone) Annotations() []entschema.Annotation {
	return []entschema.Annotation{schema.Discardable{}}
}
