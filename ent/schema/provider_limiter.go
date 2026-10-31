package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SendLimiter is the state of one Integration's Provider limit (ADR 0023): two
// token buckets, per-second and daily, in a single row so a reservation can take a
// token from both or neither in one atomic UPDATE. The row is created lazily on the
// first limited send. Each bucket stores its fill as a fraction of capacity (1 is
// full), not a token count: capacity and refill rate derive from the Integration's
// max_per_second / max_per_day at reservation time, so changing a limit takes effect
// at once and a bucket that was unlimited for a while is simply full.
type SendLimiter struct {
	ent.Schema
}

func (SendLimiter) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "send_limiters"},
	}
}

func (SendLimiter) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "send_limiters"}}
}

func (SendLimiter) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("integration_id").
			Unique().
			Immutable(),
		// Fill of the per-second bucket as of refilled_at, a fraction of its capacity
		// (max_per_second tokens). It may go below zero (a bucket in debt).
		field.Float("second_fill"),
		// Fill of the rolling 24-hour bucket as of refilled_at, a fraction of its
		// capacity (max_per_day tokens).
		field.Float("day_fill"),
		// The instant both buckets were last topped up.
		field.Time("refilled_at"),
	}
}

func (SendLimiter) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("integration", Integration.Type).
			Ref("send_limiter").
			Field("integration_id").
			Unique().
			Required().
			Immutable(),
	}
}

func (SendLimiter) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id"),
	}
}
