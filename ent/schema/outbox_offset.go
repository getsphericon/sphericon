package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// OutboxOffset is one consumer group's cursor into the domain-event outbox, the
// watermill-sql offsets table of the domain_events topic. Like OutboxMessage it is owned
// by watermill (a hand-written migration creates it) and skipped by ent's diff; ent only
// generates the client. The id is the table's consumer_group.
type OutboxOffset struct {
	ent.Schema
}

func (OutboxOffset) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "watermill_offsets_domain_events", Skip: true},
	}
}

func (OutboxOffset) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			StorageKey("consumer_group").
			Immutable(),
		// The last acknowledged offset; NULL until the group has acknowledged a message.
		field.Int64("offset_acked").
			Optional().
			Nillable(),
		// The transaction id of the last processed message, an xid8.
		field.String("last_processed_transaction_id").
			SchemaType(map[string]string{dialect.Postgres: "xid8"}),
	}
}
