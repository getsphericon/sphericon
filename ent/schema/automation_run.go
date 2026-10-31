package schema

import (
	"time"

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
		TableWithEnumChecks("automation_runs", AutomationRun{}.Fields()),
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
		// The Enrollment dies with its Contact (ON DELETE CASCADE; Erasure deletes it).
		field.Int64("contact_id"),
		// The Automation version the Enrollment began on (ADR 0049): current_step indexes
		// its steps, and the worker reads them from it, so an edit never moves the step an
		// Enrollment stands on.
		field.Int64("version_id"),
		// exited: the enrollment left early (e.g. an unsubscribe, a suppression or an exit
		// event mid-run) — distinct from completing the sequence. goal_reached: the Contact
		// did what the Automation was for (its goal event) before the steps ran out.
		field.Enum("status").
			Values("active", "completed", "failed", "exited", "goal_reached").
			Default("active"),
		// When the Enrollment began (#751): the occurred_at of the Event that triggered it, or of
		// the Event a Rule fired on when a Rule's Action enrolled. An exit or goal Event ends it
		// only when it occurred at this moment or later, so loaded or late history does not.
		field.Time("began_at").
			Default(time.Now).
			Immutable().
			Annotations(entsql.Default("CURRENT_TIMESTAMP")),
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
		edge.From("version", AutomationVersion.Type).
			Ref("runs").
			Field("version_id").
			Required().
			Unique(),
		edge.To("contact", Contact.Type).Field("contact_id").Required().Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (AutomationRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("automation_id", "contact_id").
			Unique().
			StorageKey("automation_runs_automation_id_contact_id"),
		// Erasure reaches runs by contact; the leading workspace_id also serves workspace scans.
		index.Fields("workspace_id", "contact_id"),
		// The contact foreign key's index: deleting a Contact finds its runs by it.
		index.Fields("contact_id").
			StorageKey("automation_runs_contact_id"),
		// "Does an Enrollment stand on this version" decides rewrite or supersede.
		index.Fields("version_id"),
	}
}
