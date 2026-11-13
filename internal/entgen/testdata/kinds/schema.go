// Package kinds is a schema internal/entgen's tests load: one entity of each kind, which breaks no rule.
package kinds

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/ent/schema"
)

// Workspace is the tenant root the WorkspaceMixin's edge names.
type Workspace struct{ ent.Schema }

func (Workspace) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("owned", Owned.Type),
		edge.To("archived", Archived.Type),
		edge.To("discarded", Discarded.Type),
		edge.To("singles", Single.Type),
		edge.To("partial_singles", PartialSingle.Type),
		edge.To("definitions", Definition.Type),
		edge.To("definition_versions", DefinitionVersion.Type),
	}
}

// Plain has no mixin of a kind.
type Plain struct{ ent.Schema }

func (Plain) Mixin() []ent.Mixin { return []ent.Mixin{schema.TimeMixin{}} }

type Owned struct{ ent.Schema }

func (Owned) Mixin() []ent.Mixin {
	return []ent.Mixin{schema.TimeMixin{}, schema.WorkspaceMixin{Ref: "owned"}}
}

type Archived struct{ ent.Schema }

func (Archived) Mixin() []ent.Mixin {
	return []ent.Mixin{schema.TimeMixin{}, schema.WorkspaceMixin{Ref: "archived"}, schema.ArchiveMixin{}}
}

type Discarded struct{ ent.Schema }

func (Discarded) Mixin() []ent.Mixin {
	return []ent.Mixin{schema.TimeMixin{}, schema.WorkspaceMixin{Ref: "discarded"}, schema.DiscardMixin{}, schema.RevisionMixin{}}
}

// Single holds at most one row per Workspace.
type Single struct{ ent.Schema }

func (Single) Mixin() []ent.Mixin {
	return []ent.Mixin{schema.TimeMixin{}, schema.WorkspaceMixin{Ref: "singles"}}
}

func (Single) Indexes() []ent.Index {
	return []ent.Index{index.Fields("workspace_id").Unique()}
}

// PartialSingle holds at most one row per Workspace among some of its rows only.
type PartialSingle struct{ ent.Schema }

func (PartialSingle) Mixin() []ent.Mixin {
	return []ent.Mixin{schema.TimeMixin{}, schema.WorkspaceMixin{Ref: "partial_singles"}}
}

func (PartialSingle) Fields() []ent.Field {
	return []ent.Field{field.Bool("primary").Annotations(schema.Attribute("a setting its owner edits"))}
}

func (PartialSingle) Indexes() []ent.Index {
	return []ent.Index{index.Fields("workspace_id").Unique().Annotations(entsql.IndexWhere("\"primary\""))}
}

type Definition struct{ ent.Schema }

func (Definition) Mixin() []ent.Mixin {
	return []ent.Mixin{schema.TimeMixin{}, schema.WorkspaceMixin{Ref: "definitions"}, schema.ArchiveMixin{}, schema.VersionedMixin{Version: DefinitionVersion.Type}, schema.RevisionMixin{}}
}

func (Definition) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
	}
}

func (Definition) Annotations() []entschema.Annotation {
	return []entschema.Annotation{schema.TableWithEnumChecks("definitions", schema.VersionedMixin{}.Fields())}
}

type DefinitionVersion struct{ ent.Schema }

func (DefinitionVersion) Mixin() []ent.Mixin {
	return []ent.Mixin{schema.TimeMixin{}, schema.WorkspaceMixin{Ref: "definition_versions"}, schema.VersionMixin{Owner: "definition", OwnerType: Definition.Type, Table: "definition_versions"}}
}
