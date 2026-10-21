package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// PasskeyChallenge is the record of a Passkey ceremony challenge that was spent (ADR 0052).
// A challenge travels in a signed cookie and `begin` writes nothing; the row is inserted
// when a ceremony finishes, and its unique challenge makes a replay fail. It has no User:
// a usernameless login has none yet. The daily cleanup prunes rows older than a challenge
// can live. Only internal/secondfactor reads and writes it.
type PasskeyChallenge struct {
	ent.Schema
}

func (PasskeyChallenge) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "passkey_challenges"},
	}
}

func (PasskeyChallenge) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (PasskeyChallenge) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.String("challenge").
			NotEmpty().
			Unique().
			Immutable(),
	}
}
