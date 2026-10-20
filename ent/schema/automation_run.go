package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AutomationRun is one contact's progress through an Automation. The unique
// (automation_id, contact_id) index enforces enroll-once-ever (re-enrollment is
// a deliberate non-feature for the MVP). current_step points at the next step to
// execute; resume_at is set while waiting.
type AutomationRun struct {
	ent.Schema
}

func (AutomationRun) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "automation_runs"},
	}
}

func (AutomationRun) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "automation_runs"}}
}

func (AutomationRun) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("automation_id"),
		field.Int64("contact_id").
			Annotations(ScopedRef{Entity: "Contact"}),
		// exited: the enrollment left early (e.g. an unsubscribe or suppression
		// mid-run) — distinct from completing the sequence.
		field.Enum("status").
			Values("active", "completed", "failed", "exited").
			Default("active"),
		field.Int("current_step").
			Default(0).
			NonNegative(),
		field.Time("resume_at").
			Optional().
			Nillable(),
		// Why a failed enrollment failed, set by an HTTP step (ADR 0039): the closed error
		// kind, the receiver's status for http_status, and the first 1 KiB of its answer.
		// Erased with the row (ADR 0021).
		field.Enum("failure_kind").
			Values("invalid_body", "timeout", "blocked_address", "http_status", "network").
			Optional().
			Nillable(),
		field.Int("failure_status").
			Optional().
			Nillable(),
		field.String("failure_excerpt").
			Optional().
			Nillable(),
	}
}

func (AutomationRun) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("automation", Automation.Type).
			Ref("runs").
			Field("automation_id").
			Required().
			Unique(),
	}
}

func (AutomationRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("automation_id", "contact_id").
			Unique().
			StorageKey("automation_runs_automation_id_contact_id"),
		index.Fields("workspace_id"),
	}
}
