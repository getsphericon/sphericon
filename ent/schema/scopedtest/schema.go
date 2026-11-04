// Package scopedtest is the schema of a client generated for tests only (ent/scopedtest): entities
// that take a mixin no production entity takes yet, so the scoped client the templates generate for
// it is compiled and tested before a module moves onto it. Today it is the ArchiveMixin (ADR 0100):
// Plan is archivable, audited and a Versioned definition. Nothing outside the tests of the scoped
// client imports the generated package, and no migration creates its tables: a test creates them
// in a database of its own.
package scopedtest

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"

	sch "github.com/getsphericon/sphericon/ent/schema"
)

// Workspace is the tenant root the WorkspaceMixin points at, found by its name.
type Workspace struct {
	ent.Schema
}

func (Workspace) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Immutable(),
	}
}

func (Workspace) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("plans", Plan.Type),
		edge.To("plan_versions", PlanVersion.Type),
	}
}

// Plan is an archivable, audited Versioned definition.
type Plan struct {
	ent.Schema
}

func (Plan) Annotations() []schema.Annotation {
	return []schema.Annotation{
		sch.Audited{Action: "plan", NameField: "name"},
	}
}

func (Plan) Mixin() []ent.Mixin {
	return []ent.Mixin{
		sch.TimeMixin{}, sch.WorkspaceMixin{Ref: "plans"}, sch.ArchiveMixin{},
		sch.VersionedMixin{Version: PlanVersion.Type},
	}
}

func (Plan) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Immutable(),
		field.String("name").NotEmpty(),
	}
}

// PlanVersion is a version of a Plan; its body is one string.
type PlanVersion struct {
	ent.Schema
}

func (PlanVersion) Mixin() []ent.Mixin {
	return []ent.Mixin{
		sch.TimeMixin{}, sch.WorkspaceMixin{Ref: "plan_versions"},
		sch.VersionMixin{Owner: "plan", OwnerType: Plan.Type, Table: "plan_versions"},
	}
}

func (PlanVersion) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Immutable(),
		field.String("body"),
	}
}
