package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// Operator is a platform staff identity (ADR 0008, ADR 0026), separate from User: it
// has no Membership and no Workspace edge, so the scoped client does not cover it and
// no Workspace-scoped code can return one. It is created only by the CLI and is read
// and written only from ee/operator.
type Operator struct {
	ent.Schema
}

func (Operator) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "operators"},
	}
}

func (Operator) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (Operator) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// Lower-cased, trimmed login address. An Operator who is also a customer keeps
		// a User row under the same address: the two tables never meet.
		field.String("email").
			NotEmpty().
			Unique(),
		field.String("password_hash").
			Sensitive().
			NotEmpty(),
		// The TOTP secret sealed with the instance cipher. Set while enrolment is
		// pending (first login) and once it is confirmed; empty before the first login
		// and after a CLI reset.
		field.String("totp_secret_encrypted").
			Sensitive().
			Optional(),
		// When enrolment was confirmed with a valid code. Nil with a secret set is a
		// pending enrolment, which is not a second step yet.
		field.Time("totp_confirmed_at").
			Optional().
			Nillable(),
		// The last TOTP time step a code was accepted for: a code of that step or an
		// earlier one is refused, so each code works once.
		field.Int64("totp_last_step").
			Default(0),
		// The generation of the Operator's sessions: a token carries the value current
		// at issuance and is refused once it differs.
		field.Int64("session_epoch").
			Default(0),
	}
}
