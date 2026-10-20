package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Hold is a Staff hold (ADR 0009): one reversible freeze of a Workspace's outbound
// sending, set from outside through the service endpoints under a key its setter
// chooses. Several coexist per Workspace; each lapses at its expiry, which the setter
// must renew, and the send path ignores an expired row at once (a read compares
// expires_at with the clock; nothing deletes it).
type Hold struct {
	ent.Schema
}

func (Hold) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "holds"},
		Audited{Action: "hold", NameField: "key"},
	}
}

func (Hold) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "holds"}}
}

func (Hold) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The setter's key, unique per Workspace.
		field.String("key").
			NotEmpty().
			Immutable(),
		// A machine reason code.
		field.String("reason_code").
			NotEmpty(),
		// The message for the Workspace Owner, shown as given.
		field.String("message").
			NotEmpty(),
		// Where the Workspace's Owners and Admins can resolve the hold, shown as a link
		// next to the message. Absent when the setter gave none.
		field.String("action_url").
			Optional().
			Nillable(),
		field.Time("expires_at"),
		// The name of the Service credential that last set it.
		field.String("set_by").
			NotEmpty(),
	}
}

func (Hold) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("key", "workspace_id").Unique().StorageKey("holds_key_workspace_id"),
	}
}
