package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ContactImportChunk is a part of a CSV import's file (ADR 0138): the file's bytes cut in order
// into parts of about a mebibyte, written at upload, so its job reads from the chunk its checkpoint
// names rather than the whole file. Deleted when the import ends, and with its import. Only
// internal/domain/contacts/contactimports writes it.
type ContactImportChunk struct {
	ent.Schema
}

func (ContactImportChunk) Annotations() []schema.Annotation {
	a := TableWithEnumChecks("contact_import_chunks", ContactImportChunk{}.Fields())
	a.Checks["contact_import_chunks_seq_check"] = "seq >= 0"
	return []schema.Annotation{a}
}

func (ContactImportChunk) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "contact_import_chunks"}}
}

func (ContactImportChunk) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("contact_import_id").
			Immutable(),
		// The chunk's place in the file, from 0.
		field.Int("seq").
			Immutable(),
		field.Bytes("data").
			Immutable(),
	}
}

func (ContactImportChunk) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("contact_import", ContactImport.Type).
			Ref("chunks").
			Field("contact_import_id").
			Unique().
			Required().
			Immutable(),
	}
}

func (ContactImportChunk) Indexes() []ent.Index {
	return []ent.Index{
		// A file's chunks, in order; one per place.
		index.Fields("workspace_id", "contact_import_id", "seq").
			Unique(),
	}
}
