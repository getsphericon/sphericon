package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AutomationVersion is one copy of an Automation's steps (ADR 0049, GLOSSARY: Automation
// version). An Enrollment stands on the version it began on, so editing the steps changes
// only later Enrollments. A version no Enrollment stands on is rewritten in place rather
// than superseded; the name and Trigger stay on the Automation.
type AutomationVersion struct {
	ent.Schema
}

func (AutomationVersion) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "automation_versions"},
		// The Audit entry says the steps changed and never what they say: an HTTP step
		// carries sealed secrets (ADR 0039).
		Audited{Action: "automation_version"},
	}
}

func (AutomationVersion) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "automation_versions"}}
}

func (AutomationVersion) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("automation_id"),
		// 1 for the first, plus one per version after it, within one Automation.
		field.Int("version").
			Positive().
			Immutable(),
		// JSON array of steps: [{"type":"email","subject":..,"body":<html>},
		// {"type":"wait","seconds":N}]. Sensitive: HTTP steps carry sealed URL, header
		// values and secret (ADR 0039), so the Audit entry only says the steps changed.
		field.String("steps").
			Default("[]").
			Annotations(Sensitive{}),
	}
}

func (AutomationVersion) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("automation", Automation.Type).
			Ref("versions").
			Field("automation_id").
			Required().
			Unique(),
		edge.To("runs", AutomationRun.Type),
	}
}

func (AutomationVersion) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("automation_id", "version").
			Unique().
			StorageKey("automation_versions_automation_id_version"),
	}
}
