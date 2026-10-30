package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// LegalDocumentDraft is the text a Staff member is writing for a Sign-up item's next Legal
// document version (ADR 0062): one per item, edited freely, seen by no visitor.
type LegalDocumentDraft struct {
	ent.Schema
}

func (LegalDocumentDraft) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "legal_document_drafts"},
	}
}

func (LegalDocumentDraft) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (LegalDocumentDraft) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("item_id").
			Unique().
			Immutable(),
		// Markdown.
		field.Text("markdown"),
	}
}

func (LegalDocumentDraft) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("item", SignupItem.Type).
			Ref("draft").
			Field("item_id").
			Unique().
			Required().
			Immutable(),
	}
}
