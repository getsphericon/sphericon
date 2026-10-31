package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// InstanceAuditEntry is one entry of the Instance audit log (ADR 0070): a Staff member changed a
// setting of the Instance itself, such as the default Send cap. It is the Instance's counterpart of
// a Workspace's Audit entry, and it is core: written on every Instance, whatever the License key.
// It belongs to no Workspace (no Workspace edge, no scoped wrapper), so Workspace deletion never
// erases it. internal/instanceaudit writes it in the transaction of the change and reads it back;
// nothing updates or deletes an entry.
type InstanceAuditEntry struct {
	ent.Schema
}

func (InstanceAuditEntry) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "instance_audit_entries"},
	}
}

// Mixin gives the entry its creation time: when the change was made.
func (InstanceAuditEntry) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (InstanceAuditEntry) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The Staff member who made the change.
		field.Int64("staff_member_id").
			Immutable(),
		// What was changed, as `instance.<what>_change`.
		field.String("action").
			NotEmpty().
			Immutable(),
		// The old and the new value of each changed field, keyed by its column: a "from" absent
		// where there was no value, a "to" absent where it was cleared.
		field.JSON("diff", map[string]any{}).
			Immutable(),
	}
}

func (InstanceAuditEntry) Edges() []ent.Edge {
	return []ent.Edge{
		// A Staff member is never deleted (ADR 0026), so the actor always resolves.
		edge.To("staff_member", StaffMember.Type).
			Field("staff_member_id").
			Unique().
			Required().
			Immutable().
			Annotations(entsql.OnDelete(entsql.Restrict)),
	}
}

func (InstanceAuditEntry) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("staff_member_id"),
	}
}
