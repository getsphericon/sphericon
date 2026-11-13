package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// OutboundMessage is the durable record of one Outbound send (ADR 0015): one row per
// email sent on a Workspace's behalf, whichever surface asked for it. It is claimed
// BEFORE the provider is called, under a per-logical-send idempotency key, so a
// retried job or request never sends twice; the outcome and the `email.sent` Event
// are written in one transaction afterwards. It carries provenance and outcome
// only — never the rendered content (PII + volume); the one exception is the subject line,
// kept for the feeds (see `subject`). It references the surface's
// own records by foreign keys (review rule 8): RESTRICT for a Broadcast, an
// Automation and an Integration, which are discarded and never deleted (ADR 0049), SET NULL
// for a Contact, a Template, an Automation run and a Broadcast recipient, which a physical delete or an
// Erasure removes while the message stays as a delivery record.
type OutboundMessage struct {
	ent.Schema
}

// Annotations adds to the enum checks that a message's final report is delivered or undelivered,
// never both (ADR 0079), and that it carries exactly its kind's provenance. The report needs no
// Channel condition: only an SMS report sets undelivered_at.
func (OutboundMessage) Annotations() []schema.Annotation {
	a := TableWithEnumChecks("outbound_messages", OutboundMessage{}.Fields())
	a.Checks["outbound_messages_final_report_check"] = "delivered_at IS NULL OR undelivered_at IS NULL"
	a.Checks["outbound_messages_provenance_check"] = variantCheck("kind", outboundProvenance)
	return []schema.Annotation{WithLowerCheck(a, "destination")}
}

// outboundProvenance is the surface record each kind of Outbound message points at: a Broadcast
// message its Broadcast, an Automation message its Automation and step, and neither of them the
// other's. A column whose foreign key is ON DELETE SET NULL (the recipient, the run, the Template)
// is never required: it is cleared when its row goes and the message stays. A kind added to the
// enum adds its entry here.
var outboundProvenance = []variant{
	{value: "broadcast", required: []string{"broadcast_id"}, optional: []string{"broadcast_recipient_id"}},
	{value: "automation", required: []string{"automation_id", "automation_step"}, optional: []string{"automation_run_id"}},
	{value: "transactional", optional: []string{"template_id"}},
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
			GoType(channels.Channel("")).
			Default("email"),
		// Normalized (lower-cased) channel-specific destination. Cleared (NULL) when the
		// Contact it belonged to is erased (ADR 0021): the row stays as an anonymous
		// delivery record.
		field.String("destination").
			Optional(),
		// The Contact this destination resolved to, when one exists (a transactional
		// destination may have none). Display and Event attribution only. SET NULL: the
		// message stays as an anonymous delivery record when the Contact is erased.
		field.Int64("contact_id").
			Optional().
			Nillable(),
		// The unsubscribe Sending source; nil for transactional (it carries none).
		field.String("sending_source").
			Optional().
			Nillable(),
		// Domain of the effective From address, stamped on email.* Events so the
		// deliverability rates can be grained per Sending domain (ADR 0011).
		field.String("sending_domain").
			Optional().
			Nillable(),
		// The provider's id for the accepted message (SES MessageId, SMTP Message-ID, Twilio's SID).
		// Cleared on a text when its Contact is erased: it names the number and the text in the
		// provider's log (ADR 0021).
		field.String("provider_message_id").
			Optional().
			Nillable(),
		// pending: claimed, provider not yet answered (or a crashed attempt).
		// sent: accepted by the provider. skipped: final for this destination
		// (see reason). failed: permanent failure of this one message.
		field.Enum("status").
			Annotations(Machine()).
			Values("pending", "sent", "skipped", "failed").
			Default("pending"),
		// Why the message was skipped (an eligibility Reason*) or the failure text.
		field.String("reason").
			Optional().
			Nillable(),
		// Lease: a pending claim may be taken over by a retry only when its
		// claimed_at is older than the lease, so an in-flight send is never raced.
		field.Time("claimed_at").
			Annotations(Attribute(reasonWithStatus)).
			Default(time.Now),
		field.Time("sent_at").
			Annotations(Attribute(reasonWithStatus)).
			Optional().
			Nillable(),
		// The delivery result (ADR 0053): each kind keeps the earliest Delivery notice that names
		// this message (ADR 0113), and a stamp is never cleared, so a report of an old send does
		// not shrink when its analytical Events expire (ADR 0019). An SMS final report sets
		// delivered_at once, through the latch of undelivered_at below.
		field.Time("delivered_at").Optional().Nillable().Annotations(Attribute(reasonEarliest)),
		field.Time("bounced_at").Optional().Nillable().Annotations(Attribute(reasonEarliest)),
		field.Time("complained_at").Optional().Nillable().Annotations(Attribute(reasonEarliest)),
		// A report that the message did not reach the person, on a Channel whose provider reports
		// it (SMS, ADR 0079), with the provider's error code (Twilio's ErrorCode), nil when it gave
		// none. delivered_at and undelivered_at are the final states of a sent message, one latch
		// over the pair: the first of them stamps and the other never follows, and the table
		// refuses a row with both (outbound_messages_final_report_check).
		field.Time("undelivered_at").Optional().Nillable().Annotations(Latch()),
		field.String("undelivered_code").Optional().Nillable(),
		// The recipient's first human open and first human click of the message (ADR 0053),
		// stamped once like the delivery results above, whatever surface sent it. A machine hit
		// stamps nothing.
		field.Time("opened_at").Optional().Nillable().Annotations(Attribute(reasonEarliest)),
		field.Time("clicked_at").Optional().Nillable().Annotations(Attribute(reasonEarliest)),
		// True when the sender the message went through said it delivers nowhere outside (a
		// Test inbox, ADR 0058): the message was captured, not delivered. Stamped with the
		// send from the sender's own capability, so it outlives the Integration's state and
		// every report can say so without joining to the provider.
		field.Bool("captured").Default(false).Annotations(Attribute(reasonFact)),
		// Surface provenance, foreign keys declared in Edges.
		field.Int64("broadcast_id").Optional().Nillable(),
		field.Int64("broadcast_recipient_id").Optional().Nillable(),
		field.Int64("automation_id").Optional().Nillable(),
		field.Int64("automation_run_id").Optional().Nillable(),
		field.Int("automation_step").Optional().Nillable(),
		// The Integration the message went out through (ADR 0023), its sent count over 24 hours is the
		// Integration's usage. Nil when the sender had no Integration row.
		field.Int64("integration_id").Optional().Nillable(),
		// The Template a transactional send referenced (ADR 0005).
		field.Int64("template_id").Optional().Nillable(),
		// The SHA-256 of what a Transactional request with the caller's Idempotency-Key asked for
		// (its Template, destination, variables and tracking): a request that repeats the key is
		// replayed only when it asks for the same, and refused otherwise. Nil for every other send,
		// whose key the surface derives from its own record. Derived from personal data: cleared
		// with the Contact (ADR 0021).
		field.String("request_digest").
			Optional().
			Nillable(),
		// The subject the recipient received, merge tags filled, written when an email is sent
		// so a feed can name the email an Event is about after its Broadcast or Automation step
		// changed or went. Personal data: cleared with the Contact (ADR 0021). Nil for a message
		// sent before it was kept, a Channel with no subject and a message that was not sent.
		field.String("subject").
			Optional().
			Nillable(),
		// The Instance's origin (SPHERICON_APP_URL) the message's links were built on, so
		// `sphericon doctor` can warn when the configured origin has since changed (ADR 0036).
		// Nil when the send carried no links of its own (a Transactional send has no Tracker).
		field.String("app_origin").
			Optional().
			Nillable().
			Immutable(),
	}
}

func (OutboundMessage) Edges() []ent.Edge {
	return []ent.Edge{
		// The short links of its text and buttons (ADR 0083) die with it.
		edge.To("tracked_links", TrackedLink.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("broadcast", Broadcast.Type).Field("broadcast_id").Unique().
			Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.To("automation", Automation.Type).Field("automation_id").Unique().
			Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.To("integration", Integration.Type).Field("integration_id").Unique().
			Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.To("broadcast_recipient", BroadcastRecipient.Type).Field("broadcast_recipient_id").Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.To("automation_run", AutomationRun.Type).Field("automation_run_id").Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.To("contact", Contact.Type).Field("contact_id").Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.To("template", EmailTemplate.Type).Field("template_id").Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
	}
}

func (OutboundMessage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "idempotency_key").
			Unique().
			StorageKey("outbound_messages_workspace_id_idempotency_key"),
		// Send-history listing, newest first, per surface.
		index.Fields("workspace_id", "kind", "created_at"),
		// Message retention: each kind's oldest messages across every Workspace (ADR 0143).
		index.Fields("kind", "created_at").
			StorageKey("outbound_messages_kind_created_at"),
		// Usage of an Integration over a trailing window (ADR 0023).
		index.Fields("workspace_id", "integration_id", "sent_at"),
		// Deliverability windows per Sending domain (ADR 0011).
		index.Fields("workspace_id", "sending_domain", "created_at"),
		// A Delivery notice reaches its message by the provider's message id (ADR 0053).
		index.Fields("workspace_id", "provider_message_id").
			StorageKey("outbound_messages_workspace_id_provider_message_id"),
		// The Outbound messages Export stream's cursor (ADR 0045).
		index.Fields("workspace_id", "updated_at", "id"),
		// Erasure and contact lookups reach this table by contact; the contact foreign key's
		// index (whole: a key's index cannot be partial).
		index.Fields("workspace_id", "contact_id").
			StorageKey("outbound_messages_workspace_id_contact_id"),
		// A Broadcast's delivery figures: its messages grouped by status (ADR 0015).
		index.Fields("workspace_id", "broadcast_id", "status"),
		// A Broadcast recipient's message: one per recipient, so "is this recipient still
		// pending" is an indexed lookup. Only Broadcast sends carry the column.
		index.Fields("workspace_id", "broadcast_recipient_id").
			StorageKey("outbound_messages_workspace_id_broadcast_recipient_id"),
		// The keys that need a lookup by the referenced row: its delete or its restriction check.
		index.Fields("broadcast_id").
			StorageKey("outbound_messages_broadcast_id"),
		index.Fields("integration_id").
			StorageKey("outbound_messages_integration_id"),
		index.Fields("workspace_id", "automation_id").
			StorageKey("outbound_messages_workspace_id_automation_id"),
		index.Fields("workspace_id", "automation_run_id").
			StorageKey("outbound_messages_workspace_id_automation_run_id"),
		index.Fields("workspace_id", "template_id").
			StorageKey("outbound_messages_workspace_id_template_id"),
		// The workspace analytics dashboard's send cohort: Broadcast sends by send time.
		index.Fields("workspace_id", "sent_at").
			StorageKey("outbound_messages_workspace_id_sent_at_broadcast").
			Annotations(entsql.IndexWhere("broadcast_recipient_id IS NOT NULL")),
	}
}
