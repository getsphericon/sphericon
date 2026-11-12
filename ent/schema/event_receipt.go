package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// EventReceipt remembers consumption of an externally identified fact, independently of its
// optional Event projection and of mutable consent. It contains only a scoped digest, never
// the source identifier, provider message reference or personal event payload (ADR 0114).
type EventReceipt struct{ ent.Schema }

func (EventReceipt) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "event_receipts"}}
}

func (EventReceipt) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

// Indexes backs the prune by age (events.PruneReceipts), which ends a receipt with the Event
// retention window.
func (EventReceipt) Indexes() []ent.Index {
	return []ent.Index{index.Fields("created_at")}
}

func (EventReceipt) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").StorageKey("id").Immutable(),
		field.String("receipt_key").NotEmpty().Immutable().Unique(),
	}
}
