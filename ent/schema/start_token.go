package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// StartToken is the single-use token of a Telegram start link (ADR 0076): issued for a Contact or
// a Visitor, redeemed by the `/start <token>` the bot receives. Only the hash is stored. It is
// consumed by one conditional write of used_at, and it dies with its owner and its Integration.
type StartToken struct {
	ent.Schema
}

func (StartToken) Annotations() []schema.Annotation {
	a := entsql.Annotation{Table: "start_tokens", Checks: map[string]string{"start_tokens_owner_check": ownerCheck}}
	return []schema.Annotation{a}
}

func (StartToken) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "start_tokens"}}
}

func (StartToken) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("integration_id").
			Immutable(),
		// Exactly one of the two owners (start_tokens_owner_check).
		field.Int64("contact_id").
			Optional().
			Nillable().
			Immutable(),
		field.Int64("visitor_id").
			Optional().
			Nillable().
			Immutable(),
		// SHA-256 (hex) of the token.
		field.String("token_hash").
			NotEmpty().
			Immutable().
			Sensitive(),
		field.Time("expires_at").
			Annotations(Attribute(reasonExpiry)).
			Immutable(),
		// When the token was redeemed; set once.
		field.Time("used_at").
			Annotations(Latch()).
			Optional().
			Nillable(),
	}
}

func (StartToken) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("integration", Integration.Type).
			Ref("start_tokens").
			Field("integration_id").
			Unique().
			Required().
			Immutable(),
		edge.From("contact", Contact.Type).
			Ref("start_tokens").
			Field("contact_id").
			Unique().
			Immutable(),
		edge.From("visitor", Visitor.Type).
			Ref("start_tokens").
			Field("visitor_id").
			Unique().
			Immutable(),
	}
}

func (StartToken) Indexes() []ent.Index {
	return []ent.Index{
		// The redeem finds the token by its hash in the Integration's Workspace.
		index.Fields("workspace_id", "token_hash").
			Unique().
			StorageKey("start_tokens_token_hash"),
		index.Fields("workspace_id", "integration_id").
			StorageKey("start_tokens_workspace_id_integration_id"),
		index.Fields("workspace_id", "contact_id").
			StorageKey("start_tokens_workspace_id_contact_id"),
		index.Fields("workspace_id", "visitor_id").
			StorageKey("start_tokens_workspace_id_visitor_id"),
		// The purge of lapsed tokens finds them by their expiry.
		index.Fields("expires_at").
			StorageKey("start_tokens_expires_at"),
	}
}
