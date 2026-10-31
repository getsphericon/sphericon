package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SendCapLimiter is the state of one Workspace's Send cap (ADR 0067): a rolling 24-hour token
// bucket beside its Integrations' Provider limit buckets (ADR 0023), at most one row per
// Workspace, created lazily on the first send under a Send cap. Like the Provider limit's, the
// bucket stores its fill as a fraction of capacity (1 is full): capacity and refill derive from
// the Workspace's send_cap at reservation time, so a changed Send cap takes effect at once.
type SendCapLimiter struct {
	ent.Schema
}

func (SendCapLimiter) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "send_cap_limiters"},
	}
}

func (SendCapLimiter) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "send_cap_limiters"}}
}

func (SendCapLimiter) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// Fill of the rolling 24-hour bucket as of refilled_at, a fraction of its capacity
		// (send_cap tokens). It may go below zero (a bucket in debt after Transactional sends).
		field.Float("day_fill"),
		// The instant the bucket was last topped up.
		field.Time("refilled_at"),
	}
}

func (SendCapLimiter) Indexes() []ent.Index {
	return []ent.Index{
		// One bucket per Workspace.
		index.Fields("workspace_id").Unique(),
	}
}
