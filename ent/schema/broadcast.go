package schema

import (
	"encoding/json"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Broadcast is a workspace-scoped one-off email campaign. Its lifecycle is
// draft -> scheduled -> sending -> sent (or failed). The body is authored as
// HTML; body_text is derived for the plain-text part. Audience is the segment
// when segment_id is set, otherwise all active contacts in the workspace.
// Aggregate counters are denormalized for cheap reporting; per-recipient state
// lives in BroadcastRecipient.
type Broadcast struct {
	ent.Schema
}

func (Broadcast) Annotations() []schema.Annotation {
	return []schema.Annotation{
		broadcastTable(),
		Audited{Action: "broadcast", NameField: "name"},
	}
}

func (Broadcast) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "broadcasts"}, DiscardMixin{}}
}

func (Broadcast) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.String("name").
			NotEmpty(),
		field.String("subject").
			Default(""),
		field.String("from_name").
			Optional().
			Nillable(),
		field.String("from_email").
			Optional().
			Nillable(),
		// Email-ready HTML with Liquid merge tags (ADR 0040).
		field.String("body").
			Default(""),
		// The editor's JSON document (ADR 0040), set only when the Broadcast was written as a
		// document; the server rendered body from it in the same save. A body-only write clears it.
		field.JSON("doc", json.RawMessage{}).
			Optional(),
		// The editor version that rendered doc; set and cleared with it.
		field.String("editor_version").
			Optional().
			Nillable(),
		field.String("body_text").
			Default(""),
		// Nil segment_id means "all active contacts in the workspace".
		field.Int64("segment_id").
			Optional().
			Nillable(),
		// Nil integration_id means "the workspace default email integration".
		field.Int64("integration_id").
			Optional().
			Nillable(),
		field.Enum("status").
			Values("draft", "scheduled", "sending", "sent", "failed").
			Default("draft"),
		field.Time("scheduled_at").
			Optional().
			Nillable(),
		field.Time("sent_at").
			Optional().
			Nillable(),
		// No delivery figures are stored: they are derived from the recipients and their
		// Outbound messages (ADR 0015), so they cannot drift from the rows.
		// Why the broadcast is currently held (ADR 0015): a reversible, per-source
		// hold — Workspace suspension, an unverified Sending domain, no Integration —
		// not a failure. Set while any recipient is held, cleared when one sends; the
		// status stays "sending" and the pending recipients wait for the hold to lift.
		field.String("hold_reason").
			Optional().
			Nillable(),
		// When the last paced recipient job is scheduled (ADR 0023): the planner spreads
		// recipient jobs at one over the Integration's effective rate, and the ETA is
		// derived from this and the recipients still pending. Nil when the plan was not
		// paced (no Send rate limit).
		field.Time("last_scheduled_at").
			Optional().
			Nillable(),
	}
}

func (Broadcast) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("recipients", BroadcastRecipient.Type),
		// A Segment and an Integration are discarded, never deleted (ADR 0049), so a Broadcast's
		// reference to one always resolves, as history too. Only Workspace erasure purges them,
		// after the Broadcasts.
		edge.To("segment", Segment.Type).
			Field("segment_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.To("integration", Integration.Type).
			Field("integration_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.Restrict)),
	}
}

func (Broadcast) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id"),
		index.Fields("segment_id"),
		index.Fields("integration_id"),
	}
}

// broadcastTable adds the timestamps a status promises to the enum checks: a scheduled
// Broadcast has its time, a sent one its sent_at (each set in the write that moves the status).
func broadcastTable() entsql.Annotation {
	a := TableWithEnumChecks("broadcasts", Broadcast{}.Fields())
	a.Checks["broadcasts_scheduled_at_check"] = "status <> 'scheduled' OR scheduled_at IS NOT NULL"
	a.Checks["broadcasts_sent_at_check"] = "status <> 'sent' OR sent_at IS NOT NULL"
	return a
}
