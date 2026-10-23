package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type User struct {
	ent.Schema
}

func (User) Annotations() []schema.Annotation {
	name, expr := LowerEmailCheck("users")
	return []schema.Annotation{
		entsql.Annotation{Table: "users", Checks: map[string]string{name: expr}},
	}
}

func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.String(nameField).
			NotEmpty(),
		field.String("email").
			NotEmpty().
			Unique(),
		field.String("password_hash").
			Sensitive().
			Optional(),
		// When the user confirmed ownership of their email (nullable = unverified).
		// Login is gated on it (ADR 0030): a User without it is an Unverified User, deleted
		// by the prune job once it is stale and holds no Membership.
		field.Time("email_verified_at").
			Optional().
			Nillable(),
		// When the User's address was proven to be theirs (ADR 0048): set by a Sign-in
		// provider's verified email (and, from the other flows, by a verify link, an
		// accepted Invitation and a confirmed email change). It differs from
		// email_verified_at, which an install that turned verification off sets at
		// registration without any proof; only a proven address may be linked silently.
		field.Time("email_proven_at").
			Optional().
			Nillable(),
		// The generation of the User's sessions (ADR 0020): every session token
		// carries the value current at issuance, and a token whose value differs is
		// rejected. Bumping it ends every session of the User at once.
		field.Int64("session_epoch").
			Default(0),
		// The TOTP Second factor (ADR 0020): the shared secret sealed with the
		// instance cipher (internal/secrets). Set while enrollment is pending and
		// once it is active; empty when the User has no Second factor.
		field.String("second_factor_secret_encrypted").
			Sensitive().
			Optional(),
		// When enrollment was confirmed with a valid code. Nil with a secret set is a
		// pending enrollment, which is not a Second factor yet.
		field.Time("second_factor_confirmed_at").
			Optional().
			Nillable(),
		// The last TOTP time step a code was accepted for: a code of that step or an
		// earlier one is refused, so each code works once.
		field.Int64("second_factor_last_step").
			Default(0),
		// When a public sign-up (password or Sign-in provider) marked the User as owing the
		// Sign-up survey, with the survey on. Nil is not due: an Invitation, create-owner,
		// join-domain admission, a pre-existing User, or the survey already answered. The
		// submit clears it in the transaction that creates the SignupSurveyResponse.
		field.Time("signup_survey_due_at").
			Optional().
			Nillable(),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("memberships", Membership.Type),
		edge.To("sent_invitations", Invitation.Type),
		edge.To("recovery_codes", RecoveryCode.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("passkeys", Passkey.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("identities", UserIdentity.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("signup_survey_response", SignupSurveyResponse.Type).
			Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}
