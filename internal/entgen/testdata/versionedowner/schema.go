// Package versionedowner is a schema internal/entgen's tests load: a Versioned definition whose version has no VersionMixin.
package versionedowner

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
		edge.To("definitions", Definition.Type),
		edge.To("definition_versions", DefinitionVersion.Type),
	}
}

type Definition struct{ ent.Schema }

func (Definition) Mixin() []ent.Mixin {
	return []ent.Mixin{schema.TimeMixin{}, schema.WorkspaceMixin{Ref: "definitions"}, schema.ArchiveMixin{}, schema.VersionedMixin{Version: DefinitionVersion.Type, Table: "definitions"}, schema.RevisionMixin{}}
}

func (Definition) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
	}
}

type DefinitionVersion struct{ ent.Schema }

func (DefinitionVersion) Mixin() []ent.Mixin {
	return []ent.Mixin{schema.TimeMixin{}, schema.WorkspaceMixin{Ref: "definition_versions"}}
}
