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
// verifier) for a Connection: an ApiToken of the chosen workspace tied to the approving
// User's Membership. It is Workspace-owned: the consent creates it through the scope of the
// Workspace it names, and the exchange reads it by its secret with the raw client and claims it
// through the scope of the Workspace the row names.
type OAuthCode struct {
	ent.Schema
}

func (OAuthCode) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "oauth_codes"},
	}
}

func (OAuthCode) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "oauth_codes"}}
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
		field.Time("expires_at").
			Annotations(Attribute(reasonExpiry)),
		field.Time("used_at").
			Annotations(Latch()).
			Optional().
			Nillable(),
		field.Int64("client_id"),
		// The approving User's Membership of that workspace, which the Connection belongs to.
		field.Int64("membership_id"),
	}
}

func (OAuthCode) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("client", OAuthClient.Type).
			Ref("codes").
			Field("client_id").
			Required().
			Unique(),
		edge.From("membership", Membership.Type).
			Ref("oauth_codes").
			Field("membership_id").
			Required().
			Unique(),
	}
}

func (OAuthCode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("client_id"),
		index.Fields("workspace_id", "membership_id"),
	}
}
