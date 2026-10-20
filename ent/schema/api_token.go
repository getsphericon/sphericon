package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ApiToken struct {
	ent.Schema
}

func (ApiToken) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "api_tokens"},
		Audited{Action: "api_token", NameField: "name"},
	}
}

func (ApiToken) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "api_tokens"}}
}

func (ApiToken) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.String("name").
			NotEmpty(),
		field.String("prefix").
			NotEmpty().
			Unique().
			Immutable(),
		field.String("secret_hash").
			NotEmpty().
			Sensitive().
			Annotations(Sensitive{}),
		field.JSON("scopes", []string{}).
			Default([]string{}),
		field.Time("expires_at").
			Optional().
			Nillable(),
		field.Time("revoked_at").
			Optional().
			Nillable(),
		field.Time("last_used_at").
			Optional().
			Nillable(),
	}
}

func (ApiToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id"),
	}
}
