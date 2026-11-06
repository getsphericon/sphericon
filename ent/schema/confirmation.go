package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// Confirmation is the derived read-model row for a confirmed subscription on a (workspace,
// channel, destination) (ADR 0013, 0077). It is the positive mirror of Unsubscribe: presence means
// "confirmed", and it is keyed by destination — not by contact — so the eligibility gate is a
// single fast lookup symmetric with the negative layers. The source of truth is the immutable
// marketing.confirmed Event; this row is the projection, so there is no "confirmed" flag on the
// Contact (ADR 0001/0011 pattern).
//
// On email it is consulted only when the workspace opts into confirmed opt-in; single-opt-in
// workspaces never read it. On sms it is always required (ADR 0077). A deliberate "everything"
// unsubscribe deletes the row (stale consent never silently reactivates); the Event log is
// preserved. A Confirmation is replaced only by a stronger one (confirmations.Stronger).
type Confirmation struct {
	ent.Schema
}

func (Confirmation) Annotations() []schema.Annotation {
	return []schema.Annotation{
		TableWithEnumChecks("confirmations", Confirmation{}.Fields()),
	}
}

func (Confirmation) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "confirmations"}}
}

func (Confirmation) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The channel this confirmation applies to.
		field.Enum("channel").
			GoType(channels.Channel("")).
			Default("email"),
		// Normalized channel-specific destination: the lower-cased email address, or the phone
		// number in E.164.
		field.String("destination").
			NotEmpty(),
		// How the confirmation was obtained. "double_opt_in" is the gold standard
		// (the recipient clicked the link); "grandfathered" is written by the
		// policy-enablement backfill (admin asserts a prior-consent basis);
		// "imported" is asserted by an API client or an import for consent it collected itself,
		// with its evidence; "keyword" is a START the person texted (ADR 0077).
		field.Enum("provenance").
			Values("double_opt_in", "grandfathered", "imported", "keyword"),
		// The evidence of an imported Confirmation, as its marketing.confirmed Event carries it
		// (ADR 0077): the consent wording the person was shown, where it was collected, and the
		// brand it was given to. Null on a Confirmation that carries none (a double opt-in, a
		// keyword).
		field.String("wording").
			Optional().
			Nillable(),
		field.String("source").
			Optional().
			Nillable(),
		field.String("brand").
			Optional().
			Nillable(),
		// The contact this destination belongs to, when known. Display only; the
		// confirmation is keyed by destination. It is consent proof for that person, so it
		// dies with the Contact (ON DELETE CASCADE; Erasure deletes it, ADR 0021). Nullable.
		field.Int64("contact_id").
			Optional().
			Nillable(),
	}
}

func (Confirmation) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("contact", Contact.Type).Field("contact_id").Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (Confirmation) Indexes() []ent.Index {
	return []ent.Index{
		// One confirmation per (channel, destination) per workspace; the positive
		// eligibility gate reads it, and the confirm link upserts idempotently.
		index.Fields("workspace_id", "channel", "destination").
			Unique().
			StorageKey("confirmations_ws_channel_dest"),
		// Erasure and contact lookups reach this table by contact. Not partial: it is also the
		// contact foreign key's index.
		index.Fields("contact_id").
			StorageKey("confirmations_contact_id"),
	}
}
