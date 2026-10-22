package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BroadcastRecipient is the per-recipient delivery log for a Broadcast. It is
// the basis for delivery tracking (opens/clicks) and idempotent sending: the
// unique (broadcast_id, contact_id) index guarantees one row per recipient.
type BroadcastRecipient struct {
	ent.Schema
}

func (BroadcastRecipient) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "broadcast_recipients"},
	}
}

func (BroadcastRecipient) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "broadcast_recipients"}}
}

func (BroadcastRecipient) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("broadcast_id"),
		// Cleared (NULL) when the Contact is erased (ADR 0021, ON DELETE SET NULL): the row
		// stays as an anonymous delivery record.
		field.Int64("contact_id").
			Optional(),
		// No delivery state here (ADR 0015): this row is the frozen audience snapshot plus
		// the engagement rollup. How its send ended is its Outbound message's (the message
		// carries this row's id), and it is pending until that message leaves pending.
		// When a Deferral last put this recipient's job to sleep until (ADR 0023). A
		// recipient whose job is still asleep is not queueing for a token, so the
		// backlog a deferred job waits behind counts only pending recipients that are
		// awake: no deferral recorded, or one that has passed.
		field.Time("deferred_until").
			Optional().
			Nillable(),
		field.Time("opened_at").
			Optional().
			Nillable(),
		field.Time("clicked_at").
			Optional().
			Nillable(),
		// When this recipient unsubscribed from the Broadcast's email: set once, like
		// opened_at and clicked_at, so the Broadcast's unsubscribes are a count of these.
		field.Time("unsubscribed_at").
			Optional().
			Nillable(),
	}
}

func (BroadcastRecipient) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("broadcast", Broadcast.Type).
			Ref("recipients").
			Field("broadcast_id").
			Required().
			Unique(),
		edge.To("contact", Contact.Type).Field("contact_id").Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
	}
}

func (BroadcastRecipient) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("broadcast_id", "contact_id").
			Unique().
			StorageKey("broadcast_recipients_broadcast_id_contact_id"),
		index.Fields("workspace_id"),
		// Erasure and contact lookups reach this table by contact; the contact foreign key's
		// index (whole: a key's index cannot be partial).
		index.Fields("contact_id").
			StorageKey("broadcast_recipients_contact_id"),
	}
}
