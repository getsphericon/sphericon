package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// RiverJob is river's job table (river_job), modelled as a skipped entity the way
// OutboxMessage models the outbox: river owns the table and its migrations
// (migrations/*_river_00N.sql), so Skip keeps it out of ent's diff and ent only generates
// the client. The code that removes queued work for Erasure (events.QueuePurger) uses it
// instead of SQL. Only the columns that code reads are declared; river writes the rest.
type RiverJob struct {
	ent.Schema
}

func (RiverJob) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "river_job", Skip: true},
	}
}

func (RiverJob) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			Immutable(),
		field.String("kind").
			Immutable(),
		// river's river_job_state enum: available, cancelled, completed, discarded, pending,
		// retryable, running, scheduled.
		field.String("state").
			SchemaType(map[string]string{dialect.Postgres: "river_job_state"}),
		// The job's JSON-encoded arguments.
		field.Bytes("args").
			Immutable().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
	}
}
