package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AuditEntry is one row of a Workspace's append-only Audit log (GLOSSARY, ADR 0022).
// It is an Enterprise feature (ee/LICENSE): only the ee/audit subscriber writes it, and
// only the ee/audit reader reads it. ent keeps one schema package per graph, so the
// table is declared here next to the others and migrated by the same Atlas flow; the
// license gate lives in ee/, not in the schema. No code path updates or deletes a row
// (the ee/retention prune job, ADR 0014, is the only remover).
type AuditEntry struct {
	ent.Schema
}

func (AuditEntry) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "audit_entries"},
	}
}

func (AuditEntry) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "audit_entries"}}
}

func (AuditEntry) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The bus envelope id: the dedupe key that makes at-least-once redelivery
		// write the entry once.
		field.String("entry_key").
			NotEmpty().
			Immutable(),
		field.Time("occurred_at").
			Immutable(),
		// user | api_token | staff member | system (ingest is never recorded).
		field.String("actor_kind").
			NotEmpty().
			Immutable(),
		field.String("actor_id").
			Optional().
			Nillable().
			Immutable(),
		// Snapshot of the actor's display name (a User's name), so the entry stays
		// readable after the actor is gone.
		field.String("actor_name").
			Optional().
			Nillable().
			Immutable(),
		// <entity>.<verb>, e.g. membership.update.
		field.String("action").
			NotEmpty().
			Immutable(),
		field.String("target_type").
			NotEmpty().
			Immutable(),
		field.String("target_id").
			Optional().
			Nillable().
			Immutable(),
		// Snapshot of the target's name; absent for a Contact (personal data).
		field.String("target_name").
			Optional().
			Nillable().
			Immutable(),
		// Before/after of the changed fields; sensitive fields appear as changed only.
		field.JSON("diff", map[string]any{}).
			Optional().
			Immutable(),
		field.String("request_id").
			Optional().
			Nillable().
			Immutable(),
		field.String("ip").
			Optional().
			Nillable().
			Immutable(),
		field.String("user_agent").
			Optional().
			Nillable().
			Immutable(),
	}
}

func (AuditEntry) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "entry_key").Unique().StorageKey("audit_entries_workspace_id_entry_key"),
		index.Fields("workspace_id", "id"),
	}
}
