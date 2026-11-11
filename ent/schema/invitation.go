package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Invitation is a pending offer of Membership to an email address. It carries its
// own token + expiry (before any User exists), and becomes a Membership on accept, which
// keeps the row as the latch and clears its address.
// owner is never invitable — only owner/admin manage members, and ownership is
// transferred, not invited.
type Invitation struct {
	ent.Schema
}

func (Invitation) Annotations() []schema.Annotation {
	table := TableWithEnumChecks("invitations", Invitation{}.Fields())
	name, expr := LowerCheck("invitations", "email")
	table.Checks[name] = expr
	// A pending Invitation has its address and an accepted one has none.
	table.Checks["invitations_email_check"] = "(accepted_at IS NULL) = (email IS NOT NULL)"
	return []schema.Annotation{table}
}

func (Invitation) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "invitations"}}
}

func (Invitation) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The address the offer goes to, only while it is pending: acceptance clears it with the
		// latch, since the Membership and the Audit entry are what remain of the offer, and an
		// address kept there would outlive a User who deletes themself (ADR 0050).
		field.String("email").
			NotEmpty().
			Optional().
			Nillable(),
		field.Enum("role").
			Values("admin", "member"),
		// SHA-256 of the raw invite token; the raw token is only ever in the link
		// (email + copy-link), never stored — mirrors api_token.secret_hash.
		field.String("token_hash").
			NotEmpty().
			Sensitive(),
		// The User who sent the invite (informational). Optional so an invite
		// survives the inviter's deletion.
		field.Int64("invited_by").
			Optional().
			Nillable(),
		field.Time("expires_at").
			Annotations(Attribute(reasonExpiry)),
		field.Time("accepted_at").
			Annotations(Latch()).
			Optional().
			Nillable(),
	}
}

func (Invitation) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("inviter", User.Type).
			Ref("sent_invitations").
			Field("invited_by").
			Unique(),
		// A Sign-in provider's sign-up held for the Sign-up items goes with its Invitation (ADR 0062).
		edge.To("pending_signups", PendingSignup.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (Invitation) Indexes() []ent.Index {
	return []ent.Index{
		// One unaccepted invite per address. Accepted Invitations are immutable facts;
		// inviting that address again creates a new Invitation (ADR 0109).
		index.Fields("workspace_id", "email").
			Unique().
			StorageKey("invitations_pending_workspace_email").
			Annotations(entsql.IndexWhere("accepted_at IS NULL")),
		// The pending-only key cannot support the Workspace foreign key for accepted rows.
		index.Fields("workspace_id"),
		index.Fields("invited_by"),
	}
}
