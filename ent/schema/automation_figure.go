package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AutomationFigure is what one step position of an Automation sent before message retention
// deleted its Outbound messages (ADR 0143): each pruned batch adds its counts here in the
// transaction that deletes it, so the Automation's report adds these to what its live messages
// count and reads what it read before the prune.
type AutomationFigure struct {
	ent.Schema
}

func (AutomationFigure) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "automation_figures"},
	}
}

func (AutomationFigure) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "automation_figures"}}
}

func (AutomationFigure) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("automation_id").
			Immutable(),
		// The position of the send step, as an Automation message's automation_step.
		field.Int("step").
			Immutable(),
		// The counts the report reads from the messages: sent and skipped by step, and the
		// delivery stamps.
		field.Int("sent").NonNegative().Default(0),
		field.Int("skipped").NonNegative().Default(0),
		field.Int("delivered").NonNegative().Default(0),
		field.Int("bounced").NonNegative().Default(0),
		field.Int("complained").NonNegative().Default(0),
		field.Int("captured").NonNegative().Default(0),
	}
}

func (AutomationFigure) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("automation", Automation.Type).Field("automation_id").Unique().Required().Immutable(),
	}
}

func (AutomationFigure) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "automation_id", "step").
			Unique().
			StorageKey("automation_figures_workspace_id_automation_id_step"),
	}
}
