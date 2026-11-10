package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// DiscardMixin gives an entity the nullable discarded_at stamp (ADR 0049, glossary Discard):
// the row is hidden from lists and from new work but kept, and never brought back (ADR 0100); only
// a Workspace returns, within its window. There is no discarded_by: the Audit entry names the actor.
//
// A Workspace-owned entity that embeds it gets Discard and Purge (Workspace erasure) on the scoped
// client and no Delete or Undiscard; list queries hide discarded rows (QueryWithDiscarded sees all),
// and by-id reads and UpdateOneID see every row. The Workspace takes only the field: it has no
// scoped wrapper and keeps its compare-and-swap lifecycle (ADR 0046). An entity takes either this
// mixin or ArchiveMixin, never both (ADR 0100).
//
// A unique index on a discardable entity MUST be partial,
// entsql.IndexWhere("discarded_at IS NULL"): otherwise a discarded row holds the name for ever.
//
// discarded_at is a latch (ADR 0109): the generated Discard stamps it once, conditioned on its being
// null, and nothing clears it. The Workspace's is a machine instead, its deletion cycle (ADR 0043), so
// it passes Restorable.
type DiscardMixin struct {
	mixin.Schema
	// Restorable marks the one discardable entity that comes back within its window, the Workspace.
	Restorable bool
}

func (m DiscardMixin) Fields() []ent.Field {
	form := Latch()
	if m.Restorable {
		form = Machine()
	}
	return []ent.Field{
		field.Time("discarded_at").
			Annotations(form).
			Optional().
			Nillable(),
	}
}
