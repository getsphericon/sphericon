package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
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
// A unique index or field of an archivable entity is partial, entsql.IndexWhere("archived_at IS
// NULL"), so an archived row does not hold the name for ever, or says with UniqueOverAll why it
// holds over archived rows too; internal/entgen refuses one that is neither.
type ArchiveMixin struct {
	mixin.Schema
}

func (ArchiveMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Time(ArchivedColumn).
			Annotations(Machine()).
			Optional().
			Nillable(),
	}
}

// Annotations declare the kind the mixin gives: archivable.
func (ArchiveMixin) Annotations() []schema.Annotation {
	return []schema.Annotation{Archivable{}}
}
