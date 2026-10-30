package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// LegalDocumentVersion is a published version of a Sign-up item's Legal document (ADR 0062): its
// Markdown, its label and who published it when. It is never updated or deleted: a new text is a
// new version. The item's current version is its latest.
type LegalDocumentVersion struct {
	ent.Schema
}

func (LegalDocumentVersion) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "legal_document_versions"},
	}
}

func (LegalDocumentVersion) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (LegalDocumentVersion) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("item_id").
			Immutable(),
		// The name the Staff member gave the version, unique per item.
		field.String("label").
			NotEmpty().
			MaxLen(64).
			SchemaType(map[string]string{dialect.Postgres: "varchar(64)"}).
			Immutable(),
		// Markdown.
		field.Text("markdown").
			Immutable(),
		field.Time("published_at").
			Immutable(),
		field.Int64("published_by_id").
			Immutable(),
	}
}

func (LegalDocumentVersion) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("item", SignupItem.Type).
			Ref("versions").
			Field("item_id").
			Unique().
			Required().
			Immutable(),
		// A Staff member is never deleted (ADR 0026), so the publisher always resolves.
		edge.To("published_by", StaffMember.Type).
			Field("published_by_id").
			Unique().
			Required().
			Immutable().
			Annotations(entsql.OnDelete(entsql.Restrict)),
		// The decisions Users made on the version; a version that has one is never deleted.
		edge.To("consent_records", ConsentRecord.Type).
			Annotations(entsql.OnDelete(entsql.Restrict)),
	}
}

func (LegalDocumentVersion) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("item_id", "label").
			Unique(),
		index.Fields("published_by_id"),
	}
}
