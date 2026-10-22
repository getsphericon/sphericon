package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// PrivateRelayRange is one egress address range of iCloud Private Relay, as Apple publishes
// them (ADR 0053). A request from inside one is flagged as machine engagement. The table is
// the whole published list, replaced as one by the refresh job (internal/machine); it holds
// no Workspace data, so it has no Workspace edge.
type PrivateRelayRange struct {
	ent.Schema
}

func (PrivateRelayRange) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "private_relay_ranges"},
	}
}

func (PrivateRelayRange) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The range in CIDR notation, IPv4 or IPv6, in its masked form.
		field.String("cidr").
			NotEmpty().
			Immutable().
			Unique(),
	}
}

func (PrivateRelayRange) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}
