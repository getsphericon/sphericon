package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UserIdentity is a Linked identity (ADR 0048): the pair of a Sign-in provider and the
// provider's immutable subject for one person, attached to a User. A sign-in finds the User
// by this pair, never by email, so a person who changes the address at the provider still
// signs in to the same account. It belongs to a User, not a Workspace; only
// internal/accounts reads and writes it. The subject is the provider's `sub` (Google) or
// numeric account id (GitHub), never logged or written to an Audit entry.
type UserIdentity struct {
	ent.Schema
}

func (UserIdentity) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "user_identities"},
	}
}

func (UserIdentity) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (UserIdentity) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("user_id").
			Immutable(),
		// The registry name of the Sign-in provider ("google").
		field.String("provider").
			NotEmpty().
			Immutable(),
		field.String("subject").
			Sensitive().
			NotEmpty().
			Immutable(),
	}
}

func (UserIdentity) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("identities").
			Field("user_id").
			Unique().
			Required().
			Immutable(),
	}
}

func (UserIdentity) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider", "subject").Unique(),
		index.Fields("user_id"),
	}
}
