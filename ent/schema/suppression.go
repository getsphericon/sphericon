package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Suppression is the workspace's authoritative, (channel, destination)-keyed
// do-not-send registry for hard, global facts: hard bounce, spam complaint, and
// manual bans. It is the send path's global hard floor, checked on every send,
// and a compliance ratchet (bounce/complaint do not auto-clear). Keyed by
// (channel, destination) — not by contact — so it covers destinations with no
// contact (e.g. a bounce) and survives contact deletion/re-import. The
// destination is stored normalized (lower-cased) and unique per (workspace,
// channel) among the unlifted rows, so ingestion can upsert idempotently. Distinct from Unsubscribe:
// Suppression is global and hard; an unsubscribe is per-sending-source and
// toggleable (see the Unsubscribe entity).
type Suppression struct {
	ent.Schema
}

func (Suppression) Annotations() []schema.Annotation {
	return []schema.Annotation{
		TableWithEnumChecks("suppressions", Suppression{}.Fields()),
	}
}

func (Suppression) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "suppressions"}}
}

func (Suppression) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The channel this suppression applies to (email today; sms reserved).
		field.Enum("channel").
			Values("email").
			Default("email"),
		// Normalized (lower-cased) channel-specific destination address.
		field.String("destination").
			NotEmpty(),
		// Why the destination is suppressed. Unsubscribe is NOT a suppression
		// reason — it lives in the separate, toggleable Unsubscribe entity.
		field.Enum("reason").
			Values("bounce", "complaint", "manual").
			Default("manual"),
		// The contact this destination belongs to, when known (a bounce/complaint
		// may arrive for a destination with no contact). Display only. Nullable.
		field.Int64("contact_id").
			Optional().
			Nillable().
			Annotations(ScopedRef{Unchecked: true}),
		// When the Suppression was lifted (ADR 0049): the row stays as the record of why the
		// destination once refused mail, and no longer blocks sends. Null while it is active.
		field.Time("lifted_at").
			Optional().
			Nillable(),
	}
}

func (Suppression) Indexes() []ent.Index {
	return []ent.Index{
		// One active suppression per (channel, destination) per workspace; lets ingestion
		// upsert (the conflict target repeats the predicate). A lifted row keeps its place
		// in the table but not the name, so the destination can be suppressed anew.
		index.Fields("workspace_id", "channel", "destination").
			Unique().
			Annotations(entsql.IndexWhere("lifted_at IS NULL")).
			StorageKey("suppressions_workspace_id_channel_destination"),
		// Erasure and contact lookups reach this table by contact. Not partial: the unique index
		// above is, so this one is also the workspace_id foreign key's index.
		index.Fields("workspace_id", "contact_id").
			StorageKey("suppressions_workspace_id_contact_id"),
	}
}
