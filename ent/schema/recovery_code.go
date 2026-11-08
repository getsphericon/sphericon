package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// RecoveryCode is one single-use code that stands in for a TOTP code when the
// User's authenticator is lost (ADR 0020). Only its SHA-256 hash is stored: the
// plaintext is shown once, when the set is generated. Regenerating replaces the
// whole set. It belongs to a User, not a Workspace; only
// internal/domain/identity/secondfactors, the owner of the Second factor, reads and writes it.
type RecoveryCode struct {
	ent.Schema
}

func (RecoveryCode) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "recovery_codes"},
	}
}

func (RecoveryCode) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (RecoveryCode) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("user_id").
			Immutable(),
		field.String("code_hash").
			Sensitive().
			NotEmpty().
			Immutable(),
		// When the code was spent; nil while it is still usable.
		field.Time("used_at").
			Optional().
			Nillable(),
	}
}

func (RecoveryCode) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("recovery_codes").
			Field("user_id").
			Unique().
			Required().
			Immutable(),
	}
}

func (RecoveryCode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "code_hash").Unique(),
	}
}
