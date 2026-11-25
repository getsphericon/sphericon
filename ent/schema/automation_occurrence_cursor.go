package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AutomationOccurrenceCursor is how far the date scheduler has decided the occurrences of an
// Automation's date Trigger (ADR 0126), kept beside the Automation as a Webhook endpoint's health
// is beside it (ADR 0168): the scheduler writes it every minute with no actor, so it is not audited
// and moves no revision of the Automation. One row per Automation, reset by each Publish of a date
// Trigger: its span starts and its watermark stands at the Publish.
type AutomationOccurrenceCursor struct {
	ent.Schema
}

func (AutomationOccurrenceCursor) Annotations() []schema.Annotation {
	return []schema.Annotation{
		TableWithEnumChecks("automation_occurrence_cursors", AutomationOccurrenceCursor{}.Fields()),
	}
}

func (AutomationOccurrenceCursor) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "automation_occurrence_cursors"}}
}

func (AutomationOccurrenceCursor) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("automation_id").
			Immutable(),
		// The start of the active span of the live version (decision 1 of #1613): the later of the
		// Automation's activation and the Publish of its live version, on the Instance clock. No
		// occurrence before it enrolls anyone or is missed.
		field.Time("span_start"),
		// The watermark: the moment, on the Instance clock, up to which the occurrences are decided.
		// It starts at span_start and moves to each tick's now.
		field.Time("decided_through"),
		// The Workspace time zone the last tick read the occurrences in; nil before the first tick
		// and for a Workspace whose zone is unset (UTC). A tick that finds another zone rescans the
		// span of the last 24 hours for every Contact.
		field.String("workspace_zone").
			Optional().
			Nillable(),
		// The latest change of a Contact of the Workspace (its updated_at, the clock that stamps the
		// row) the last tick had seen: the next tick reads the dates set since. Nil before the first
		// tick, which reads every Contact.
		field.Time("contacts_seen_through").
			Optional().
			Nillable(),
	}
}

func (AutomationOccurrenceCursor) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("automation", Automation.Type).
			Ref("occurrence_cursor").
			Field("automation_id").
			Unique().
			Required().
			Immutable(),
	}
}

func (AutomationOccurrenceCursor) Indexes() []ent.Index {
	return []ent.Index{
		// One cursor per Automation; it serves the Automation's composite foreign key too.
		index.Fields("workspace_id", "automation_id").
			Unique().
			StorageKey(OccurrenceCursorIndex),
	}
}

// OccurrenceCursorIndex is the unique index of an Automation's occurrence cursor.
const OccurrenceCursorIndex = "automation_occurrence_cursors_automation"
