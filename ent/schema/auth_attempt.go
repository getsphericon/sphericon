package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AuthAttempt is the failed-attempt counter of one normalized email for one action
// kind (ADR 0025). It follows the account, not the source IP, so a distributed
// guessing attack on one account hits one counter, and it lives in Postgres so the
// count is exact across replicas. It has no Workspace (a User is not Workspace-owned)
// and no edge to User: rows are written for unknown emails too, so the table cannot
// be used to enumerate accounts. Only internal/accounts and ee/operator (through
// accounts.Attempts) read and write it. An Operator's failures count under their own
// kind, so they never share a counter with a User of the same address.
type AuthAttempt struct {
	ent.Schema
}

func (AuthAttempt) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "auth_attempts"},
	}
}

func (AuthAttempt) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (AuthAttempt) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// Lower-cased, trimmed address, whether or not an account exists.
		field.String("email").
			NotEmpty().
			Immutable(),
		field.Enum("kind").
			Values("login", "operator_login", "password_reset").
			Immutable(),
		// Failures since the window opened.
		field.Int("failures").
			Default(0),
		// The most recent failure: the window and the delay are measured from it.
		field.Time("last_attempt_at"),
	}
}

func (AuthAttempt) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("email", "kind").Unique(),
		// The purge job scans by age.
		index.Fields("last_attempt_at"),
	}
}
