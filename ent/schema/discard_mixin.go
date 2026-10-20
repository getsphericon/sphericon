package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// DiscardMixin gives an entity the nullable discarded_at stamp (ADR 0049, glossary Discard):
// the row is hidden from lists and from new work but kept, and Undiscard clears the stamp.
// There is no discarded_by: the Audit entry (discard, restore) names the actor.
//
// A Workspace-owned entity that embeds it gets Discard, Undiscard and Purge on the scoped
// client and no Delete; list queries hide discarded rows (QueryWithDiscarded sees all), and
// by-id reads and UpdateOneID see every row. The Workspace takes only the field: it has no
// scoped wrapper and keeps its compare-and-swap lifecycle (ADR 0046).
//
// A unique index on a discardable entity MUST be partial,
// entsql.IndexWhere("discarded_at IS NULL"): otherwise a discarded row holds the name for ever.
type DiscardMixin struct {
	mixin.Schema
}

func (DiscardMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Time("discarded_at").
			Optional().
			Nillable(),
	}
}
