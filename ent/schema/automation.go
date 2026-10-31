package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Automation is a workspace-scoped, event-triggered workflow and a Versioned definition
// (VersionedMixin): its definition (Trigger, Integration and steps) lives in its versions (ADR
// 0049, ADR 0065). A contact is enrolled into the live version when an Event whose action equals
// that version's trigger_event is recorded for them, and the run walks the steps of the version it
// began on. Edits write the draft, the one version above live, which Publish makes live. The
// status, activated_at and the two pointers come from the mixin.
type Automation struct {
	ent.Schema
}

func (Automation) Annotations() []schema.Annotation {
	return []schema.Annotation{
		TableWithEnumChecks("automations", append(Automation{}.Fields(), VersionedMixin{}.Fields()...)),
		Audited{Action: "automation", NameField: nameField},
	}
}

func (Automation) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{}, WorkspaceMixin{Ref: "automations"}, DiscardMixin{},
		VersionedMixin{Version: AutomationVersion.Type},
	}
}

func (Automation) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.String(nameField).
			NotEmpty(),
	}
}

func (Automation) Edges() []ent.Edge {
	return []ent.Edge{
		// Purging an Automation (only one with no Enrollments, ADR 0049) purges its Enrollments
		// with it; an Automation with history is discarded instead.
		edge.To("runs", AutomationRun.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		// The mail the Automation sent: facts that outlive their Enrollments (Contact erasure
		// removes those), so the row's key is RESTRICT and a purge needs none left. Declared for
		// the predicate (HasOutboundMessages); the key stays on OutboundMessage.
		edge.From("outbound_messages", OutboundMessage.Type).
			Ref("automation"),
	}
}

func (Automation) Indexes() []ent.Index {
	return []ent.Index{
		// The Workspace foreign key's index. The Trigger lookup goes through the versions.
		index.Fields("workspace_id"),
	}
}
