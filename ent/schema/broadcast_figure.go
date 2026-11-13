package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BroadcastFigure is a finished Broadcast's figures, frozen once before message retention deletes
// its recipients and Outbound messages (ADR 0143). It is written once and never recomputed: a
// Broadcast with this row reports it and nothing live, even while its rows are still being deleted.
type BroadcastFigure struct {
	ent.Schema
}

func (BroadcastFigure) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "broadcast_figures"},
	}
}

func (BroadcastFigure) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "broadcast_figures"}}
}

func (BroadcastFigure) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("broadcast_id").
			Immutable(),
		// The counts of the Stats of internal/domain/orchestration/broadcasts, but its Revenue (read
		// from the Orders, which retention keeps). Pending is the recipients a failed Broadcast never
		// sent (ADR 0046); none on a sent one.
		field.Int("recipients").NonNegative().Immutable(),
		field.Int("unsubscribed").NonNegative().Immutable(),
		field.Int("sent").NonNegative().Immutable(),
		field.Int("skipped").NonNegative().Immutable(),
		field.Int("failed").NonNegative().Immutable(),
		field.Int("pending").NonNegative().Immutable(),
		field.Int("delivered").NonNegative().Immutable(),
		field.Int("undelivered").NonNegative().Immutable(),
		field.Int("bounced").NonNegative().Immutable(),
		field.Int("complained").NonNegative().Immutable(),
		field.Int("opened").NonNegative().Immutable(),
		field.Int("clicked").NonNegative().Immutable(),
		field.Int("captured").NonNegative().Immutable(),
		// When the last of the Broadcast's recipients and messages was deleted; nil while the prune
		// is still under way.
		field.Time("pruned_at").
			Annotations(Latch()).
			Optional().
			Nillable(),
	}
}

func (BroadcastFigure) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("broadcast", Broadcast.Type).Ref("figures").Field("broadcast_id").Unique().Required().Immutable(),
	}
}

func (BroadcastFigure) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "broadcast_id").
			Unique().
			StorageKey("broadcast_figures_workspace_id_broadcast_id"),
	}
}
