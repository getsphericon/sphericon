package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Order is the read model of a shop's order Events (ADR 0064): one per (Workspace, order_id).
// It holds who and in which currency, never an amount: revenue is the sum of its Movements.
type Order struct {
	ent.Schema
}

func (Order) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "orders"},
	}
}

func (Order) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "orders"}}
}

func (Order) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The shop's own id, as the spec's `order_id` carries it. The spec has no provider, so
		// two shops feeding one Workspace keep their ids apart.
		field.String("order_id").
			NotEmpty().
			Immutable(),
		// The Contact the Event resolved to, null for an Order of nobody known. A nullable FK
		// with ON DELETE SET NULL, like an Event's: Erasure removes the identity, and the
		// amounts stay, so past revenue does not shrink.
		field.Int64("contact_id").
			Optional().
			Nillable(),
		// ISO 4217 code, in capitals: the `currency` of the Order Completed, which must name one.
		// A later Event that names none is in this currency.
		field.String("currency").
			NotEmpty().
			Immutable(),
		// The credit (ADR 0064): the Broadcast or Automation whose email the Contact last clicked
		// inside the Attribution window, and the time of that click. Found once when the Order is
		// created and never looked for again; all three are null for an unattributed Order, and
		// at most one of the two references is set.
		field.Int64("broadcast_id").
			Optional().
			Nillable().
			Immutable(),
		field.Int64("automation_id").
			Optional().
			Nillable().
			Immutable(),
		field.Time("credited_click_at").
			Optional().
			Nillable().
			Immutable(),
	}
}

func (Order) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("contact", Contact.Type).
			Ref("orders").
			Field("contact_id").
			Unique(),
		// A Broadcast and an Automation are discarded, never deleted (ADR 0049), so the credit
		// always resolves; RESTRICT, like an Outbound message's.
		edge.To("broadcast", Broadcast.Type).
			Field("broadcast_id").
			Unique().
			Immutable().
			Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.To("automation", Automation.Type).
			Field("automation_id").
			Unique().
			Immutable().
			Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.To("movements", OrderMovement.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (Order) Indexes() []ent.Index {
	return []ent.Index{
		// One Order per (Workspace, order_id): a repeat of an Order Completed changes nothing.
		index.Fields("workspace_id", "order_id").
			Unique(),
		// The contact foreign key's index: deleting a Contact finds its Orders by it.
		index.Fields("contact_id"),
		// The credit's foreign keys' indexes, and the revenue of one Broadcast or Automation.
		index.Fields("broadcast_id"),
		index.Fields("automation_id"),
	}
}

// OrderMovement is one signed change of an Order's revenue on one day (ADR 0064): its total when
// completed, the difference of an update, minus a refund or what a cancellation takes back.
// Revenue is always a sum of Movements, and a Movement counts on the Report day of its own time,
// so a figure already shown for a past day does not move.
type OrderMovement struct {
	ent.Schema
}

func (OrderMovement) Annotations() []schema.Annotation {
	return []schema.Annotation{
		TableWithEnumChecks("order_movements", OrderMovement{}.Fields()),
	}
}

func (OrderMovement) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "order_movements"}}
}

func (OrderMovement) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("order_id").
			Immutable(),
		field.Enum("kind").
			Values("completed", "updated", "refunded", "cancelled").
			Immutable(),
		// Signed, in the minor unit of the Order's currency.
		field.Int64("amount").
			Immutable(),
		// The time of the Event that caused it (its `occurredAt`).
		field.Time("occurred_at").
			Immutable(),
		// The `refund_id` of an Order Refunded, when it sent one: a repeat of it is a no-op.
		field.String("refund_id").
			NotEmpty().
			Immutable().
			Optional().
			Nillable(),
		// The id of the Event that caused it (the one bus delivery is deduplicated by), so a
		// redelivered Event is read once. Null for the Order Completed, which the Order's own
		// unique key already deduplicates.
		field.String("event_id").
			NotEmpty().
			Immutable().
			Optional().
			Nillable(),
	}
}

func (OrderMovement) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("order", Order.Type).
			Ref("movements").
			Field("order_id").
			Required().
			Immutable().
			Unique(),
	}
}

func (OrderMovement) Indexes() []ent.Index {
	return []ent.Index{
		// Backs the revenue reads: the movements of one Workspace in a window of time.
		index.Fields("workspace_id", "occurred_at"),
		// The order foreign key's index: an Order's Movements, and the cascade.
		index.Fields("order_id"),
		// A refund_id refunds once per order: a resent refund finds its Movement.
		index.Fields("workspace_id", "order_id", "refund_id").
			Unique().
			Annotations(entsql.IndexWhere("refund_id IS NOT NULL")),
		// An Event is read once, however often the bus delivers it.
		index.Fields("workspace_id", "event_id").
			Unique().
			Annotations(entsql.IndexWhere("event_id IS NOT NULL")),
	}
}

// HeldOrderEvent is an order Event kept to be read later (ADR 0064), as sent: the four
// properties Sphericon reads, never the rest. An Order Updated, Refunded or Cancelled that arrived
// before the Order Completed of its order_id waits here and is applied, in the time order of the
// Events, when the Order is created, and is then deleted; one that never gets its Completed counts
// nothing. It is kept as sent, not as an amount: one that names no currency is read in its
// Order's, which is not known yet. An Order Completed is never held: it names its currency and is
// read at once.
type HeldOrderEvent struct {
	ent.Schema
}

func (HeldOrderEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		TableWithEnumChecks("held_order_events", HeldOrderEvent{}.Fields()),
	}
}

func (HeldOrderEvent) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "held_order_events"}}
}

func (HeldOrderEvent) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The shop's order_id; not a reference, the Order does not exist yet.
		field.String("order_id").
			NotEmpty().
			Immutable(),
		field.Enum("kind").
			Values("updated", "refunded", "cancelled").
			Immutable(),
		// The `order_id`, `total`, `currency` and `refund_id` the Event sent, as sent.
		field.JSON("properties", map[string]any{}).
			Immutable(),
		// The id of the Event, as on a Movement.
		field.String("event_id").
			NotEmpty().
			Immutable(),
		field.Time("occurred_at").
			Immutable(),
	}
}

func (HeldOrderEvent) Indexes() []ent.Index {
	return []ent.Index{
		// An Event is held once, however often the bus delivers it.
		index.Fields("workspace_id", "event_id").
			Unique(),
		// The Order Completed finds what waits for it.
		index.Fields("workspace_id", "order_id"),
	}
}
