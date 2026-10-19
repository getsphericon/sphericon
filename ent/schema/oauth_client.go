package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// OAuthClient is an MCP client that registered itself through dynamic client
// registration (RFC 7591) or whose client_id is an HTTPS URL of a Client ID Metadata
// Document (ADR 0016). All clients are public (PKCE, no secret).
type OAuthClient struct {
	ent.Schema
}

func (OAuthClient) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "oauth_clients"},
	}
}

func (OAuthClient) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (OAuthClient) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// Public identifier handed to the client at registration.
		field.String("client_id").
			NotEmpty().
			Unique().
			Immutable(),
		// Self-declared by the (untrusted) client; shown on the consent screen as text.
		field.String("name").
			NotEmpty(),
		field.JSON("redirect_uris", []string{}),
		// Set only for a Client ID Metadata Document client (client_id is its URL): the
		// name and redirect URIs are then a cache of the document, fresh until this time.
		field.Time("metadata_expires_at").
			Optional().
			Nillable(),
	}
}

func (OAuthClient) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("codes", OAuthCode.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("refresh_tokens", OAuthRefreshToken.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}
