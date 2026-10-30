package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Automation is a workspace-scoped, event-triggered workflow. Its definition (Trigger,
// Integration and steps) lives in its versions (ADR 0049, ADR 0065): a contact is enrolled into
// the live version when an Event whose action equals that version's trigger_event is recorded
// for them, and the run walks the steps of the version it began on. Edits write the draft, the
// one version above live, which Publish makes live.
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
		// active: enrolls Contacts; inactive: enrolls nobody. Publish makes an Automation active,
		// Deactivate inactive (ADR 0065).
		field.Enum("status").
			Values("active", "inactive").
			Default("inactive"),
		// What the Automation does: the version new Enrollments begin on. Nil until it is first
		// published (ADR 0065).
		field.Int64("live_version_id").
			Optional().
			Nillable(),
		// The unpublished changes: the one version above live, rewritten in place until it is
		// published or reverted. Nil when there are none.
		field.Int64("draft_version_id").
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
		// The definition lives in the versions (ADR 0049, ADR 0065). Purging an Automation purges
		// its versions with it.
		edge.To("versions", AutomationVersion.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		// The mail the Automation sent: facts that outlive their Enrollments (Contact erasure
		// removes those), so the row's key is RESTRICT and a purge needs none left. Declared for
		// the predicate (HasOutboundMessages); the key stays on OutboundMessage.
		edge.From("outbound_messages", OutboundMessage.Type).
			Ref("automation"),
		// The pointers into the Automation's own versions. SET NULL breaks the cycle with the
		// versions' key on the Automation: a purge cascades the versions, Workspace erasure deletes
		// them first.
		edge.To("live_version", AutomationVersion.Type).
			Field("live_version_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.To("draft_version", AutomationVersion.Type).
			Field("draft_version_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
	}
}

func (Automation) Indexes() []ent.Index {
	return []ent.Index{
		// The Workspace foreign key's index. The Trigger lookup goes through the versions.
		index.Fields("workspace_id"),
		index.Fields("live_version_id"),
		index.Fields("draft_version_id"),
	}
}
