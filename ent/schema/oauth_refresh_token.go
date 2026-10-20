package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OAuthRefreshToken is a single-use refresh token of an MCP connector's grant. The refresh_token
// grant claims it, rotates the grant's ApiToken secret and writes the next one; a used row stays
// so that presenting it again is recognised as reuse and revokes the grant.
type OAuthRefreshToken struct {
	ent.Schema
}

func (OAuthRefreshToken) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "oauth_refresh_tokens"},
	}
}

func (OAuthRefreshToken) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (OAuthRefreshToken) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// SHA-256 of the token; the plaintext is only ever in the token response.
		field.String("token_hash").
			NotEmpty().
			Unique().
			Immutable().
			Sensitive(),
		field.Time("expires_at"),
		field.Time("used_at").
			Optional().
			Nillable(),
		field.Int64("client_id"),
		// The grant: the one ApiToken that stays the connector's row in Settings → API keys.
		field.Int64("api_token_id"),
	}
}

func (OAuthRefreshToken) Indexes() []ent.Index {
	return []ent.Index{
		// A refresh prunes the grant's expired rows by its token.
		index.Fields("api_token_id"),
		index.Fields("client_id"),
	}
}

func (OAuthRefreshToken) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("client", OAuthClient.Type).
			Ref("refresh_tokens").
			Field("client_id").
			Required().
			Unique(),
		edge.To("api_token", ApiToken.Type).
			Field("api_token_id").
			Required().
			Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}
