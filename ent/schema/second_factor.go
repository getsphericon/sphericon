package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// SecondFactor is a User's TOTP Second factor (ADR 0020): its secret, whether it is confirmed and
// the last time step a code was accepted for. It belongs to the User, not a Workspace; only
// internal/domain/identity/secondfactors reads and writes it (ADR 0110). The row is created by the
// User's first enrollment and stays when the factor is disabled or reset, its secret cleared, so
// the enrollment identity never repeats for the User.
type SecondFactor struct {
	ent.Schema
}

func (SecondFactor) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "second_factors"},
	}
}

func (SecondFactor) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (SecondFactor) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("user_id").
			Immutable().
			Unique(),
		// The shared secret sealed with the instance cipher (internal/kernel/secrets). Set while
		// enrollment is pending and once it is active; empty when the User has no Second factor.
		field.String("secret_encrypted").
			Sensitive().
			Optional(),
		// The enrollment identity: it advances for each newly issued secret, independently of the
		// User's session epoch, since displaying a QR code does not end sessions.
		field.Int64("generation").
			NonNegative(),
		// When enrollment was confirmed with a valid code. Nil with a secret set is a pending
		// enrollment, which is not a Second factor yet.
		field.Time("confirmed_at").
			Optional().
			Nillable(),
		// The last TOTP time step a code was accepted for: a code of that step or an earlier one is
		// refused, so each code works once.
		field.Int64("last_step").
			Default(0),
	}
}

func (SecondFactor) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("second_factor").
			Field("user_id").
			Unique().
			Required().
			Immutable(),
	}
}
