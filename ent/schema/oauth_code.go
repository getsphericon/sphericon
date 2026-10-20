package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OAuthCode is a short-lived, single-use authorization code minted when a user
// approves the consent screen. The token endpoint exchanges it (with the PKCE
// verifier) for an ordinary scoped ApiToken in the chosen workspace.
type OAuthCode struct {
	ent.Schema
}

func (OAuthCode) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "oauth_codes"},
	}
}

func (OAuthCode) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (OAuthCode) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// SHA-256 of the code; the plaintext is only ever in the redirect.
		field.String("code_hash").
			NotEmpty().
			Unique().
			Immutable().
			Sensitive(),
		field.String("redirect_uri").
			NotEmpty().
			Immutable(),
		field.String("code_challenge").
			NotEmpty().
			Immutable(),
		field.JSON("scopes", []string{}).
			Default([]string{}),
		field.Time("expires_at"),
		field.Time("used_at").
			Optional().
			Nillable(),
		field.Int64("client_id"),
		// The workspace the minted token will belong to (chosen on the consent screen).
		field.Int64("workspace_id"),
	}
}

func (OAuthCode) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("client", OAuthClient.Type).
			Ref("codes").
			Field("client_id").
			Required().
			Unique(),
	}
}

func (OAuthCode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("client_id"),
	}
}
