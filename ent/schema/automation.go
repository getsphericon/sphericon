package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Automation is a workspace-scoped, event-triggered workflow. A contact is
// enrolled when an Event whose action equals trigger_event is recorded for them;
// the run then walks the linear list of steps in definition (JSON). Branch/goal
// nodes and a visual builder come later.
type Automation struct {
	ent.Schema
}

func (Automation) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "automations"},
		Audited{Action: "automation", NameField: "name"},
	}
}

func (Automation) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "automations"}}
}

func (Automation) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.String("name").
			NotEmpty(),
		field.Enum("status").
			Values("draft", "active").
			Default("draft"),
		// The Event.action that enrolls a contact (e.g. "contact.created",
		// "email.opened", a custom collect event).
		field.String("trigger_event").
			NotEmpty(),
		// JSON array of steps: [{"type":"email","subject":..,"body":<html>},
		// {"type":"wait","seconds":N}].
		// Sensitive: HTTP steps carry sealed URL, header values and secret (ADR 0039),
		// so the Audit entry only says the definition changed.
		field.String("definition").
			Default("[]").
			Annotations(Sensitive{}),
	}
}

func (Automation) Edges() []ent.Edge {
	return []ent.Edge{
		// Deleting an Automation deletes its Enrollments with it.
		edge.To("runs", AutomationRun.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (Automation) Indexes() []ent.Index {
	return []ent.Index{
		// Trigger lookup: active automations for a given event action.
		index.Fields("workspace_id", "status", "trigger_event"),
	}
}
