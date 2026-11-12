package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Tag is a named, workspace-scoped, presence-only label applied to Contacts (see
// GLOSSARY). It carries no type and no value — a Contact has the Tag or does not —
// which is what separates it from a CustomField. Auto-created on first use. A Tag is
// an attribute, never a target set: "everyone tagged X" is a Segment has-Tag rule.
type Tag struct {
	ent.Schema
}

func (Tag) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "tags"},
		Audited{Action: "tag", NameField: nameField},
	}
}

func (Tag) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "tags"}}
}

func (Tag) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The label itself, as first written; unique per workspace by its key. It originates outside the workspace
		// (imports, the API), so it is untrusted text (ADR 0016).
		field.String(nameField).
			NotEmpty(),
		// The name as Tags are matched (tagname.Key): trimmed and lower-cased, so "VIP" and "vip"
		// are one Tag, which keeps the name it was first written with. Set once, by tags.EnsureIn.
		field.String("name_key").
			NotEmpty().
			Immutable().
			Annotations(Derived{}),
	}
}

func (Tag) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("contacts", Contact.Type).
			Ref("tags").
			Through("contact_tags", ContactTag.Type),
	}
}

func (Tag) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "name_key").Unique().StorageKey("tags_workspace_id_name_key"),
	}
}
