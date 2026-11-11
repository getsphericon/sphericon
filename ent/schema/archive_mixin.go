package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// ArchiveMixin gives an entity the nullable archived_at stamp (ADR 0100, glossary Archive): work
// a person puts aside and may come back to. The row leaves the working lists and new work, an
// archived view lists it, and Unarchive brings it back; it is never deleted by its owner. There is
// no archived_by: the Audit entry (archive, unarchive) names the actor.
//
// A Workspace-owned entity that embeds it gets Archive, Unarchive, QueryArchived,
// QueryWithArchived and Purge on the scoped client and no Delete; Query() hides archived rows,
// and Get and UpdateOneID see every row, because the system still writes an archived row. An
// entity takes either this mixin or DiscardMixin, never both (ADR 0100).
//
// A unique index on an archivable entity MUST be partial,
// entsql.IndexWhere("archived_at IS NULL"): otherwise an archived row holds the name for ever.
type ArchiveMixin struct {
	mixin.Schema
}

func (ArchiveMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Time("archived_at").
			Annotations(Machine()).
			Optional().
			Nillable(),
	}
}
