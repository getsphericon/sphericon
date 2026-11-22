package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AutomationFigure is what one send Step of an Automation version sent before message retention
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
		// The version and the Step id of the send Step, as an Automation message's
		// automation_version_id and automation_step_id (ADR 0122).
		field.Int64("version_id").
			Immutable(),
		field.String("step_id").
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
		// A figure stands for the Automation messages of the version it replaced, so the version is
		// kept while one names it, as it is while a message does (RESTRICT).
		edge.To("version", AutomationVersion.Type).Field("version_id").Unique().Required().Immutable().
			Annotations(entsql.OnDelete(entsql.Restrict)),
	}
}

func (AutomationFigure) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "automation_id", "version_id", "step_id").
			Unique().
			StorageKey("automation_figures_workspace_automation_version_step"),
		// The version foreign key's index.
		index.Fields("workspace_id", "version_id").
			StorageKey("automation_figures_workspace_id_version_id"),
	}
}
