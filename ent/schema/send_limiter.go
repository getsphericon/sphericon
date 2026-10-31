package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SendLimiter is the state of one Workspace's Send cap (ADR 0067): the rolling 24-hour token
// bucket that every Outbound send of the Workspace spends from beside its Integration's
// Provider limit buckets (ProviderLimiter). One row per Workspace, created lazily on the first
// send under a Send cap. Like ProviderLimiter it stores the bucket's fill as a fraction of its
// capacity, so a Send cap a Staff member changes takes effect at once.
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
		// Fill of the rolling 24-hour bucket as of refilled_at, a fraction of its capacity (the
		// Send cap). It may go below zero (a bucket in debt after Transactional sends).
		field.Float("day_fill"),
		// The instant the bucket was last topped up.
		field.Time("refilled_at"),
	}
}

func (SendLimiter) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id").Unique(),
	}
}
