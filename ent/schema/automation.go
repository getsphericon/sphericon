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
// the run then walks the linear list of steps of the Automation version it began on.
// Branch/goal nodes and a visual builder come later.
type Automation struct {
	ent.Schema
}

func (Automation) Annotations() []schema.Annotation {
	return []schema.Annotation{
		TableWithEnumChecks("automations", Automation{}.Fields()),
		Audited{Action: "automation", NameField: nameField},
	}
}

func (Automation) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "automations"}, DiscardMixin{}}
}

func (Automation) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.String(nameField).
			NotEmpty(),
		field.Enum("status").
			Values("draft", "active").
			Default("draft"),
		// The Event.action that enrolls a contact (e.g. "contact.created",
		// "email.opened", a custom collect event).
		field.String("trigger_event").
			NotEmpty(),
		// Nil integration_id means "the workspace default email integration".
		field.Int64("integration_id").
			Optional().
			Nillable(),
	}
}

func (Automation) Edges() []ent.Edge {
	return []ent.Edge{
		// Purging an Automation (only one with no Enrollments, ADR 0049) purges its Enrollments
		// with it; an Automation with history is discarded instead.
		edge.To("runs", AutomationRun.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		// The steps live in the versions (ADR 0049); the latest one is the Automation's
		// current steps. Purging an Automation purges its versions with it.
		edge.To("versions", AutomationVersion.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		// The mail the Automation sent: facts that outlive their Enrollments (Contact erasure
		// removes those), so the row's key is RESTRICT and a purge needs none left. Declared for
		// the predicate (HasOutboundMessages); the key stays on OutboundMessage.
		edge.From("outbound_messages", OutboundMessage.Type).
			Ref("automation"),
		// An Integration is discarded, never deleted (ADR 0049), so the reference always
		// resolves. Only Workspace erasure purges it, after the Automations.
		edge.To("integration", Integration.Type).
			Field("integration_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.Restrict)),
	}
}

func (Automation) Indexes() []ent.Index {
	return []ent.Index{
		// Trigger lookup: active automations for a given event action.
		index.Fields("workspace_id", "status", "trigger_event"),
		index.Fields("integration_id"),
	}
}
