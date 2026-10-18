package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// OutboxMessage is one row of the domain-event outbox, the watermill-sql messages table of
// the domain_events topic (ADR 0019). watermill owns the table's shape: its primary key is
// (transaction_id, offset) and the subscribers' query depends on that index order, which an
// ent id cannot express, so the migration (migrations/*_domain_events_outbox.sql) is
// written by hand. Skip keeps it out of ent's own diff; ent only generates the client, so
// the code and the tests that read or prune the outbox use it instead of SQL. The id is the
// table's "offset".
type OutboxMessage struct {
	ent.Schema
}

func (OutboxMessage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "watermill_domain_events", Skip: true},
	}
}

func (OutboxMessage) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("offset").
			Immutable(),
		field.String("uuid").
			Immutable(),
		// timestamptz: InitSchema converts watermill's plain TIMESTAMP.
		field.Time("created_at"),
		// The JSON-encoded events.Envelope.
		field.Bytes("payload").
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "json"}),
		field.Bytes("metadata").
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "json"}),
		// The producing transaction's id, an xid8 that orders the rows for the subscribers.
		field.String("transaction_id").
			Immutable().
			SchemaType(map[string]string{dialect.Postgres: "xid8"}),
	}
}
