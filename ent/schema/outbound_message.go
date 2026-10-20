package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OutboundMessage is the durable record of one Outbound send (ADR 0015): one row per
// email sent on a Workspace's behalf, whichever surface asked for it. It is claimed
// BEFORE the provider is called, under a per-logical-send idempotency key, so a
// retried job or request never sends twice; the outcome and the `email.sent` Event
// are written in one transaction afterwards. It carries provenance and outcome
// only — never the rendered content (PII + volume) — and references the surface's
// own records by plain id snapshots (no FKs: the record outlives a deleted
// template, broadcast or automation).
type OutboundMessage struct {
	ent.Schema
}

func (OutboundMessage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "outbound_messages"},
	}
}

func (OutboundMessage) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "outbound_messages"}}
}

func (OutboundMessage) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// Which surface asked for the send.
		field.Enum("kind").
			Values("broadcast", "automation", "transactional").
			Immutable(),
		// Unique per (workspace, logical send): the surface builds it from its own
		// identity (broadcast recipient, automation run + step, client key). The
		// unique index below makes a duplicate send impossible.
		field.String("idempotency_key").
			NotEmpty().
			Immutable(),
		field.Enum("channel").
			Values("email").
			Default("email"),
		// Normalized (lower-cased) channel-specific destination. Cleared (NULL) when the
		// Contact it belonged to is erased (ADR 0021): the row stays as an anonymous
		// delivery record.
		field.String("destination").
			Optional(),
		// The Contact this destination resolved to, when one exists (a transactional
		// destination may have none). Display and Event attribution only.
		field.Int64("contact_id").
			Optional().
			Nillable().
			Annotations(ScopedRef{Entity: "Contact"}),
		// The unsubscribe Sending source; nil for transactional (it carries none).
		field.String("sending_source").
			Optional().
			Nillable(),
		// Domain of the effective From address, stamped on email.* Events so the
		// deliverability rates can be grained per Sending domain (ADR 0011).
		field.String("sending_domain").
			Optional().
			Nillable(),
		// The provider's id for the accepted message (SES MessageId, SMTP Message-ID).
		field.String("provider_message_id").
			Optional().
			Nillable(),
		// pending: claimed, provider not yet answered (or a crashed attempt).
		// sent: accepted by the provider. skipped: final for this destination
		// (see reason). failed: permanent failure of this one message.
		field.Enum("status").
			Values("pending", "sent", "skipped", "failed").
			Default("pending"),
		// Why the message was skipped (an eligibility Reason*) or the failure text.
		field.String("reason").
			Optional().
			Nillable(),
		// Lease: a pending claim may be taken over by a retry only when its
		// claimed_at is older than the lease, so an in-flight send is never raced.
		field.Time("claimed_at").
			Default(time.Now),
		field.Time("sent_at").
			Optional().
			Nillable(),
		// Surface provenance, plain id snapshots.
		field.Int64("broadcast_id").Optional().Nillable().
			Annotations(ScopedRef{Entity: "Broadcast"}),
		field.Int64("broadcast_recipient_id").Optional().Nillable().
			Annotations(ScopedRef{Entity: "BroadcastRecipient"}),
		field.Int64("automation_id").Optional().Nillable().
			Annotations(ScopedRef{Entity: "Automation"}),
		field.Int64("automation_run_id").Optional().Nillable().
			Annotations(ScopedRef{Entity: "AutomationRun"}),
		field.Int("automation_step").Optional().Nillable(),
		// The Integration the message went out through (ADR 0023), a plain id snapshot
		// like the other provenance ids: its sent count over 24 hours is the
		// Integration's usage. Nil when the sender had no Integration row.
		field.Int64("integration_id").Optional().Nillable().
			Annotations(ScopedRef{Entity: "Integration"}),
		// The Template a transactional send referenced (ADR 0005).
		field.Int64("template_id").Optional().Nillable().
			Annotations(ScopedRef{Entity: "EmailTemplate"}),
		// The Instance's origin (SPHERICON_APP_URL) the message's links were built on, so
		// `sphericon doctor` can warn when the configured origin has since changed (ADR 0036).
		// Nil when the send carried no links of its own (a Transactional send has no Tracker).
		field.String("app_origin").
			Optional().
			Nillable().
			Immutable(),
	}
}

func (OutboundMessage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "idempotency_key").
			Unique().
			StorageKey("outbound_messages_workspace_id_idempotency_key"),
		// Send-history listing, newest first, per surface.
		index.Fields("workspace_id", "kind", "created_at"),
		// Usage of an Integration over a trailing window (ADR 0023).
		index.Fields("workspace_id", "integration_id", "sent_at"),
		// Deliverability windows per Sending domain (ADR 0011).
		index.Fields("workspace_id", "sending_domain", "created_at"),
		// The Outbound messages Export stream's cursor (ADR 0045).
		index.Fields("workspace_id", "updated_at", "id"),
		// A Broadcast's delivery figures: its messages grouped by status (ADR 0015).
		index.Fields("workspace_id", "broadcast_id", "status"),
		// A Broadcast recipient's message: one per recipient, so "is this recipient still
		// pending" is an indexed lookup. Only Broadcast sends carry the column.
		index.Fields("broadcast_recipient_id").
			StorageKey("outbound_messages_broadcast_recipient_id").
			Annotations(entsql.IndexWhere("broadcast_recipient_id IS NOT NULL")),
	}
}
