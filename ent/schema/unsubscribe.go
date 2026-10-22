package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Unsubscribe is a per-(workspace, channel, destination, sending source) opt-out
// from marketing. It is keyed by destination — not by contact — so it survives
// contact deletion and re-import (the GDPR-erasure case: the opt-out is the
// minimum data retained to honor the refusal). Absence means subscribed: there
// is no positive subscription row and no "subscribed" flag on the Contact;
// resubscribing deletes the row.
//
// The sending source is automatic, never hand-authored — it IS the sender:
// "broadcasts" (shared by every broadcast), "automation:<id>" (one per
// automation), and the reserved "everything" scope (the deliberate "leave
// entirely" opt-out). A per-source unsubscribe never silently loses the whole
// contact. Distinct from Suppression: an unsubscribe is per-source and
// toggleable, where Suppression is global and a compliance ratchet.
type Unsubscribe struct {
	ent.Schema
}

func (Unsubscribe) Annotations() []schema.Annotation {
	return []schema.Annotation{
		TableWithEnumChecks("unsubscribes", Unsubscribe{}.Fields()),
	}
}

func (Unsubscribe) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "unsubscribes"}}
}

func (Unsubscribe) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The channel this opt-out applies to (email today; sms reserved).
		field.Enum("channel").
			Values("email").
			Default("email"),
		// Normalized (lower-cased) channel-specific destination address.
		field.String("destination").
			NotEmpty(),
		// The sending source the destination opted out of: "broadcasts",
		// "automation:<id>", or the reserved "everything".
		field.String("sending_source").
			NotEmpty(),
		// The contact this destination belongs to, when known. Display only;
		// the opt-out is keyed by destination and outlives the contact (ON DELETE SET NULL;
		// Erasure clears it and the refusal stays). Nullable.
		field.Int64("contact_id").
			Optional().
			Nillable(),
	}
}

func (Unsubscribe) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("contact", Contact.Type).Field("contact_id").Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
	}
}

func (Unsubscribe) Indexes() []ent.Index {
	return []ent.Index{
		// One opt-out per (channel, destination, sending source) per workspace;
		// lets the unsubscribe link upsert idempotently. Its leading columns also
		// serve the layered eligibility check and display by (channel, destination).
		index.Fields("workspace_id", "channel", "destination", "sending_source").
			Unique().
			StorageKey("unsubscribes_ws_channel_dest_source"),
		// Erasure and contact lookups reach this table by contact; the contact foreign key's
		// index (whole: a key's index cannot be partial).
		index.Fields("contact_id").
			StorageKey("unsubscribes_contact_id"),
	}
}
