package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Workspace struct {
	ent.Schema
}

func (Workspace) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "workspaces"},
	}
}

func (Workspace) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (Workspace) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.String("name").
			NotEmpty(),
		field.String("slug").
			NotEmpty().
			Unique(),
		field.String("collect_key").
			NotEmpty().
			Unique().
			Sensitive(),
		// Secret per-workspace key that routes inbound provider webhooks
		// (/hooks/{ingest_key}/{provider}, e.g. SES bounce/complaint via SNS).
		// Distinct from the browser-exposed collect_key.
		field.String("ingest_key").
			NotEmpty().
			Unique().
			Sensitive(),
		// Confirmed opt-in policy (ADR 0013). When true, marketing sends require a
		// Confirmation for the destination (double opt-in); when false (the default),
		// the subtractive single-opt-in model applies unchanged. Forward-looking:
		// enabling it backfills grandfathered confirmations rather than muting the
		// existing list. Schema-only for now — exposed on the /site API in a later slice.
		field.Bool("require_confirmed_opt_in").
			Default(false),
		// Physical postal address printed in the marketing email footer, as
		// CAN-SPAM 15 U.S.C. §7704(a)(5) requires of every commercial message.
		// Freeform so it carries the legal sender name + address on one field
		// (the "one powerful primitive" convention). Optional and non-gating for
		// now: a workspace can still send without it — whether an empty address
		// should hard-block marketing sends (like the verified-domain gate) is a
		// separate send-path ADR. Transactional mail (ADR 0005) is exempt and
		// never renders this footer.
		field.String("postal_address").
			Optional().
			Default(""),
		// Workspace suspension (ADR 0007): a reversible freeze of ALL outbound sending
		// for reputation protection. Set => every Outbound send returns a Hold. Login,
		// reads and /collect keep working. Attribution records who set it (the
		// automated abuse detector, actor "system", or a platform Operator's id) and
		// why, so the owner sees the notice and can appeal.
		// EE advanced retention (ADR 0014, ADR 0022): audit entries older than this many
		// days are pruned by ee/retention, and only under a license that includes the
		// retention feature. NULL keeps them forever. Core never reads it.
		field.Int("retention_days").
			Optional().
			Nillable().
			Positive(),
		// Two-factor requirement (ADR 0020): when an Owner or Admin switched it on;
		// NULL while off. Each member's grace ends secondfactor.GracePeriod after the
		// later of this and their Membership's creation.
		field.Time("second_factor_required_at").
			Optional().
			Nillable(),
		field.Time("suspended_at").
			Optional().
			Nillable(),
		// Who set it: the structured actor of ADR 0026. The kind is system (the automated
		// abuse detector), cli (the operator command) or operator; only an operator has an
		// id (the real staff identity, never shown to the customer).
		field.Enum("suspended_by_kind").
			Values("system", "cli", "operator").
			Optional().
			Nillable(),
		field.String("suspended_by_id").
			Optional().
			Nillable(),
		field.String("suspension_reason").
			Optional().
			Nillable(),
	}
}

func (Workspace) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("contacts", Contact.Type),
		edge.To("custom_fields", CustomField.Type),
		edge.To("tags", Tag.Type),
		edge.To("segments", Segment.Type),
		edge.To("events", Event.Type),
		edge.To("visitors", Visitor.Type),
		edge.To("api_tokens", ApiToken.Type),
		edge.To("integrations", Integration.Type),
		edge.To("sending_domains", SendingDomain.Type),
		edge.To("send_limiters", SendLimiter.Type),
		edge.To("broadcasts", Broadcast.Type),
		edge.To("broadcast_recipients", BroadcastRecipient.Type),
		edge.To("email_templates", EmailTemplate.Type),
		edge.To("automations", Automation.Type),
		edge.To("automation_runs", AutomationRun.Type),
		edge.To("webhook_endpoints", WebhookEndpoint.Type),
		edge.To("suppressions", Suppression.Type),
		edge.To("unsubscribes", Unsubscribe.Type),
		edge.To("confirmations", Confirmation.Type),
		edge.To("outbound_messages", OutboundMessage.Type),
		edge.To("memberships", Membership.Type),
		edge.To("invitations", Invitation.Type),
		edge.To("audit_entries", AuditEntry.Type),
	}
}
