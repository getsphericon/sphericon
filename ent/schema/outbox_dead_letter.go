package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// OutboxDeadLetter is a domain event a consumer group's handler failed on past its retries: the
// watermill-sql messages table of the domain_events_dead_letters topic, which the router's poison
// queue publishes to. Like OutboxMessage, watermill owns the table's shape, the migration
// (migrations/*_domain_events_dead_letters.sql) is written by hand and Skip keeps it out of ent's
// diff; ent generates the client the replay, the gauge and the Erasure purge read it through.
// The metadata names the group (events.MetadataConsumerGroup) and the error.
type OutboxDeadLetter struct {
	ent.Schema
}

func (OutboxDeadLetter) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "watermill_domain_events_dead_letters", Skip: true},
	}
}

func (OutboxDeadLetter) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("offset").
			Immutable(),
		field.String("uuid").
			Immutable(),
		field.Time("created_at"),
		// The JSON-encoded events.Envelope, as the outbox row carried it.
		field.Bytes("payload").
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "json"}),
		// The message metadata: the consumer group, the error and where it failed.
		field.Bytes("metadata").
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "json"}),
		field.String("transaction_id").
			Immutable().
			SchemaType(map[string]string{dialect.Postgres: "xid8"}),
	}
}
