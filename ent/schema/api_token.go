package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ApiToken is a bearer credential of the external /api surface (and /mcp). Without a Membership
// it is the Workspace's API token, minted by hand with tokens:manage, which outlives the people
// who made it. With one it is a Connection (GLOSSARY, ADR 0061): the access a User gave an OAuth
// client by consenting, which can do no more than the User's current Role allows and ends with
// the Membership (removal, Leaving, User deletion) through the cascade of membership_id.
type ApiToken struct {
	ent.Schema
}

func (ApiToken) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "api_tokens"},
		Audited{Action: "api_token", NameField: nameField},
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
		field.String(nameField).
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
		// The Membership of the User who approved the Connection; null for an API token.
		field.Int64("membership_id").
			Optional().
			Nillable().
			Immutable(),
	}
}

func (ApiToken) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("membership", Membership.Type).
			Ref("connections").
			Field("membership_id").
			Unique().
			Immutable(),
	}
}

func (ApiToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id"),
		index.Fields("membership_id"),
	}
}
