package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ContactTag is the edge table of the Contact-Tag many-to-many: a presence row with no id of its
// own and no timestamps. It carries the Workspace of both its ends, so its references to them are
// composite foreign keys (ADR 0017) and the database refuses a Tag of another Workspace. The
// Contact's tags edge reads it; it is written as a row of its own, through the scoped client,
// since the edge's setters cannot fill workspace_id.
type ContactTag struct {
	ent.Schema
}

func (ContactTag) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "contact_tags"},
		field.ID("contact_id", "tag_id"),
	}
}

func (ContactTag) Mixin() []ent.Mixin {
	return []ent.Mixin{WorkspaceMixin{Ref: "contact_tags"}}
}

func (ContactTag) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("contact_id"),
		field.Int64("tag_id"),
	}
}

func (ContactTag) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("contact", Contact.Type).
			Field("contact_id").
			Required().
			Unique().
			StorageKey(edge.Symbol("contact_tags_contact_id")).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("tag", Tag.Type).
			Field("tag_id").
			Required().
			Unique().
			StorageKey(edge.Symbol("contact_tags_tag_id")).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (ContactTag) Indexes() []ent.Index {
	return []ent.Index{
		// Each composite key has an index that leads with its columns (ADR 0017); ent keys an edge
		// table on its two edge columns alone, so the primary key cannot be the one for contact_id.
		index.Fields("workspace_id", "contact_id"),
		index.Fields("workspace_id", "tag_id"),
	}
}
