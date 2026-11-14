package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// DefaultSendCap is the one row that holds the default Send cap (ADR 0067, ADR 0146): the Send cap
// every new Workspace starts with, a rule of the Workspace's standing that
// internal/domain/identity/suspension owns beside the Send cap itself. It belongs to the Instance,
// not a Workspace (no Workspace edge, no scoped wrapper). Row id 1 is made by the first change of
// the default; without it no default is set.
type DefaultSendCap struct {
	ent.Schema
}

func (DefaultSendCap) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "default_send_cap"},
	}
}

func (DefaultSendCap) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (DefaultSendCap) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// PerDay is the default as a Staff member set it: messages per rolling 24 hours, copied
		// onto a Workspace at its creation. NULL: new Workspaces get no Send cap. It applies only on
		// an Instance that has Send caps; elsewhere a stored value is ignored. Its bounds are
		// declared in the contract.
		field.Int("per_day").
			Optional().
			Nillable().
			Positive(),
	}
}
