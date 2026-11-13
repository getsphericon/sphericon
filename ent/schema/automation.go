package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Automation is a workspace-scoped, event-triggered workflow and a Versioned definition
// (VersionedMixin): its definition (Trigger, Integration and steps) lives in its versions (ADR
// 0049, ADR 0065). It is archived, never deleted (ArchiveMixin, ADR 0100). A contact is enrolled into the live version when an Event whose action equals
// that version's trigger_event is recorded for them, and the run walks the steps of the version it
// began on. Edits write the draft, the one version above live, which Publish makes live. The
// status, activated_at and the two pointers come from the mixin.
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
	return []ent.Mixin{
		TimeMixin{}, WorkspaceMixin{Ref: "automations"}, ArchiveMixin{},
		VersionedMixin{Version: AutomationVersion.Type, Table: "automations"}, RevisionMixin{},
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
		// An Automation is never deleted by its owner (ADR 0100); Workspace erasure, which
		// purges it, purges its Enrollments with it.
		edge.To("runs", AutomationRun.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}
