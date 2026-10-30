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
// who made it. With one it is a User's own credential (GLOSSARY, ADR 0061), which can do no more
// than the User's current Role allows and ends with the Membership (removal, Leaving, User
// deletion) through the cascade of membership_id: a Connection when it names the OAuth client the
// User consented to, a Personal token (a Connection without the application) when it names none.
type ApiToken struct {
	ent.Schema
}

func (ApiToken) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "api_tokens", Checks: map[string]string{
			// A Connection is a User's: an OAuth client implies a Membership.
			"api_tokens_oauth_client_check": "oauth_client_id IS NULL OR membership_id IS NOT NULL",
		}},
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
		// The Membership of the User whose Connection or Personal token it is; null for the
		// Workspace's API token.
		field.Int64("membership_id").
			Optional().
			Nillable().
			Immutable(),
		// The OAuth client the User approved, for a Connection; null for a Personal token and for
		// the Workspace's API token. It needs a Membership (api_tokens_oauth_client_check).
		field.Int64("oauth_client_id").
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
		// A Connection ends with the client it was approved for.
		edge.From("oauth_client", OAuthClient.Type).
			Ref("connections").
			Field("oauth_client_id").
			Unique().
			Immutable(),
	}
}

func (ApiToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id"),
		index.Fields("membership_id"),
		index.Fields("oauth_client_id"),
	}
}
