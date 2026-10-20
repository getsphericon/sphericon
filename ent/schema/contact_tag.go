package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ContactTag is the edge table of the Contact-Tag many-to-many: a pure presence row with
// no id of its own and no timestamps. It is a schema only so the table can carry the
// index on tag_id that deleting a Tag needs; it has no Workspace column, since both its
// ends are Workspace-owned, and is written only through the Contact's tags edge.
type ContactTag struct {
	ent.Schema
}

func (ContactTag) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "contact_tags"},
		field.ID("contact_id", "tag_id"),
	}
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
		index.Fields("tag_id"),
	}
}
