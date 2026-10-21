package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Confirmation is the derived read-model row for a confirmed (double opt-in)
// subscription on a (workspace, channel, destination) (ADR 0013). It is the
// positive mirror of Unsubscribe: presence means "confirmed", and it is keyed by
// destination — not by contact — so the eligibility gate is a single fast lookup
// symmetric with the negative layers. The source of truth is the immutable
// marketing.confirmed Event; this row is the projection, so there is no
// "confirmed" flag on the Contact (ADR 0001/0011 pattern).
//
// Confirmation is transient and one-time: an address confirmed shortly after
// acquisition operates under the normal subtractive model thereafter. It is only
// consulted when the workspace opts into confirmed-opt-in; single-opt-in
// workspaces never read it. A deliberate "everything" unsubscribe deletes the
// row (stale consent never silently reactivates); the Event log is preserved.
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
		// The channel this confirmation applies to (email today; sms reserved).
		field.Enum("channel").
			Values("email").
			Default("email"),
		// Normalized (lower-cased) channel-specific destination address.
		field.String("destination").
			NotEmpty(),
		// How the confirmation was obtained. "double_opt_in" is the gold standard
		// (the recipient clicked the link); "grandfathered" is written by the
		// policy-enablement backfill (admin asserts a prior-consent basis);
		// "imported" is asserted by the CSV importer for an existing relationship.
		field.Enum("provenance").
			Values("double_opt_in", "grandfathered", "imported"),
		// The contact this destination belongs to, when known. Display only; the
		// confirmation is keyed by destination and outlives the contact. Nullable.
		field.Int64("contact_id").
			Optional().
			Nillable().
			Annotations(ScopedRef{Unchecked: true}),
	}
}

func (Confirmation) Indexes() []ent.Index {
	return []ent.Index{
		// One confirmation per (channel, destination) per workspace; the positive
		// eligibility gate reads it, and the confirm link upserts idempotently.
		index.Fields("workspace_id", "channel", "destination").
			Unique().
			StorageKey("confirmations_ws_channel_dest"),
		// Erasure and contact lookups reach this table by contact.
		index.Fields("workspace_id", "contact_id").
			StorageKey("confirmations_workspace_id_contact_id").
			Annotations(entsql.IndexWhere("contact_id IS NOT NULL")),
	}
}
