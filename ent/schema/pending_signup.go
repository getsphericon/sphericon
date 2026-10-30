package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PendingSignup is a Sign-in provider's sign-up held until the person answers the Sign-up items
// (ADR 0062): what the provider proved (the subject, a trusted email, a name) and the Invitation
// the flow began from, kept for a few minutes and no User yet. Submitting the answers creates
// the User, its Linked identity and first Workspace from it and deletes the row; declining
// deletes it; a job prunes the expired ones. It belongs to no User and no Workspace; only
// internal/accounts reads and writes it. The subject is never logged.
type PendingSignup struct {
	ent.Schema
}

func (PendingSignup) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "pending_signups"},
	}
}

func (PendingSignup) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (PendingSignup) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The registry name of the Sign-in provider ("google").
		field.String("provider").
			NotEmpty().
			Immutable(),
		field.String("subject").
			Sensitive().
			NotEmpty().
			Immutable(),
		// The provider's trusted email, the address the User gets.
		field.String("email").
			NotEmpty().
			Immutable(),
		// The provider's name of the person, "" when it gave none.
		field.String("name").
			Default("").
			Immutable(),
		// The pending Invitation the sign-up accepts on submit, none for a plain sign-up.
		field.Int64("invitation_id").
			Optional().
			Nillable().
			Immutable(),
		// When the answers can no longer be submitted.
		field.Time("expires_at").
			Immutable(),
	}
}

func (PendingSignup) Edges() []ent.Edge {
	return []ent.Edge{
		// A withdrawn Invitation takes the sign-up made for it along: it never becomes a plain one.
		edge.From("invitation", Invitation.Type).
			Ref("pending_signups").
			Field("invitation_id").
			Unique().
			Immutable(),
	}
}

func (PendingSignup) Indexes() []ent.Index {
	return []ent.Index{
		// One pending sign-up per provider account: a new sign-in replaces it.
		index.Fields("provider", "subject").Unique(),
		index.Fields("invitation_id"),
		index.Fields("expires_at"),
	}
}
