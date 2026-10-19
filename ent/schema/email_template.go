package schema

import (
	"encoding/json"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// EmailTemplate is a workspace-scoped, reusable email body. Broadcasts copy from
// a template (no FK) so editing or deleting a template never affects a sent or
// in-flight broadcast.
type EmailTemplate struct {
	ent.Schema
}

func (EmailTemplate) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "email_templates"},
		Audited{Action: "template", NameField: "name"},
	}
}

func (EmailTemplate) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "email_templates"}}
}

func (EmailTemplate) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.String("name").
			NotEmpty(),
		field.String("subject").
			Default(""),
		// Email-ready HTML with Liquid merge tags (ADR 0040).
		field.String("body").
			Default(""),
		// The editor's JSON document (ADR 0040), set only when the Template was written as a
		// document; the server rendered body from it in the same save. A body-only write clears it.
		field.JSON("doc", json.RawMessage{}).
			Optional(),
		// The editor version that rendered doc; set and cleared with it.
		field.String("editor_version").
			Optional().
			Nillable(),
	}
}

func (EmailTemplate) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id"),
	}
}
