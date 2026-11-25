package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ProviderLimiter is the state of one Integration's Provider limit (ADR 0023): two
// token buckets, per-second and daily, in a single row so a reservation can take a
// token from both or neither in one atomic UPDATE. The row is created lazily on the
// first limited send. Each bucket stores the messages it holds (tokens); its capacity and
// refill rate derive from the Integration's max_per_second / max_per_day at reservation
// time, so a changed limit takes effect at once. The balance is never rescaled: the time
// since refilled_at refills at the limit in force at the reservation, capped at it (ADR 0023).
type ProviderLimiter struct {
	ent.Schema
}

func (ProviderLimiter) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "provider_limiters"},
	}
}

func (ProviderLimiter) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "provider_limiters"}}
}

func (ProviderLimiter) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("integration_id").
			Unique().
			Immutable(),
		// Messages the per-second bucket holds as of refilled_at, at most the max_per_second in
		// force at that write (a limit lowered since leaves it above until the next one). It may
		// go below zero (a bucket in debt). NULL is a full bucket: one not spent from yet, or
		// not limited at its last write.
		field.Float("second_tokens").
			Optional().
			Nillable(),
		// Messages the rolling 24-hour bucket holds as of refilled_at, at most the max_per_day in
		// force at that write; above it, below zero and NULL as for second_tokens.
		field.Float("day_tokens").
			Optional().
			Nillable(),
		// The instant both buckets were last topped up.
		field.Time("refilled_at").
			Annotations(Attribute(reasonBucket)),
	}
}

func (ProviderLimiter) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("integration", Integration.Type).
			Ref("provider_limiter").
			Field("integration_id").
			Unique().
			Required().
			Immutable(),
	}
}

func (ProviderLimiter) Indexes() []ent.Index {
	return []ent.Index{
		// Serves both foreign keys: the Workspace's and the Integration's composite one.
		index.Fields("workspace_id", "integration_id"),
	}
}
