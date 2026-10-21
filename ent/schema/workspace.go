package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Workspace struct {
	ent.Schema
}

func (Workspace) Annotations() []schema.Annotation {
	return []schema.Annotation{
		workspaceTable(),
	}
}

func (Workspace) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, DiscardMixin{}}
}

func (Workspace) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// Free text, never empty while the Workspace lives (the handlers refuse an empty name);
		// the erasure job blanks it on the tombstone (ADR 0043), so the schema allows it.
		field.String("name"),
		// The Workspace id (ADR 0043): 8 random base62 characters, the Workspace's one public
		// address. Assigned at creation, never changed, unique; internal/workspaceid makes it.
		field.String("public_id").
			MinLen(8).
			MaxLen(8).
			Unique().
			Immutable(),
		field.String("collect_key").
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
		// automated abuse detector, actor "system", or a platform Staff member's id) and
		// why, so the owner sees the notice and can appeal.
		// EE advanced retention (ADR 0014, ADR 0022): audit entries older than this many
		// days are pruned by ee/retention, and only under a license that includes the
		// retention feature. NULL keeps them forever. Core never reads it.
		field.Int("retention_days").
			Optional().
			Nillable().
			Positive(),
		// Domain join (ADR 0051): whether Users who prove an address on one of the Workspace's
		// verified Join domains become its members. Off until an Owner switches it on.
		field.Bool("domain_join_enabled").
			Default(false),
		// Two-factor requirement (ADR 0020): when an Owner or Admin switched it on;
		// NULL while off. Each member's grace ends secondfactor.GracePeriod after the
		// later of this and their Membership's creation.
		field.Time("second_factor_required_at").
			Optional().
			Nillable(),
		// The Workspace's place in the service feed (ADR 0009): a position that only
		// grows, taken whenever the Workspace is created or an owner is added, removed,
		// transferred or changes email or verification. 0 until the first change. Set
		// only by internal/workspacefeed.
		field.Int64("feed_position").
			Default(0),
		// Workspace deletion (ADR 0043): discarded_at (DiscardMixin) starts the 30-day window, in
		// which the Workspace is frozen, hidden and restorable; erased_at is set when the erasure
		// job has removed its data and left the row as a tombstone. Both NULL for a live Workspace.
		field.Time("erased_at").
			Optional().
			Nillable(),
		field.Time("suspended_at").
			Optional().
			Nillable(),
		// Who set it: the structured actor of ADR 0026. The kind is system (the automated
		// abuse detector), cli (a command run on the server) or staff; only a staff member has an
		// id (the real staff identity, never shown to the customer).
		field.Enum("suspended_by_kind").
			Values("system", "cli", "staff").
			Optional().
			Nillable(),
		field.String("suspended_by_id").
			Optional().
			Nillable(),
		field.String("suspension_reason").
			Optional().
			Nillable(),
		// When a Staff member last lifted a suspension (ADR 0026): the abuse detector counts
		// only Events after it, so a cleared sender is not frozen again for old data.
		field.Time("cleared_at").
			Optional().
			Nillable(),
		// Set and cleared only by a Staff member: an exempt Workspace is neither warned nor
		// frozen by the abuse detector (it can still be suspended by hand).
		field.Bool("abuse_exempt").
			Default(false),
		// When the owners were last told of the current warning / of an automatic freeze
		// (ADR 0026). The warning marker is cleared once the Workspace leaves the
		// warning tier, which is what makes one email per episode.
		field.Time("abuse_warning_notified_at").
			Optional().
			Nillable(),
		field.Time("abuse_freeze_notified_at").
			Optional().
			Nillable(),
	}
}

func (Workspace) Indexes() []ent.Index {
	return []ent.Index{
		// The feed pages by (feed_position, id).
		index.Fields("feed_position", "id"),
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
		edge.To("join_domains", JoinDomain.Type),
		edge.To("send_limiters", SendLimiter.Type),
		edge.To("holds", Hold.Type),
		edge.To("broadcasts", Broadcast.Type),
		edge.To("broadcast_recipients", BroadcastRecipient.Type),
		edge.To("email_templates", EmailTemplate.Type),
		edge.To("brand_kits", BrandKit.Type),
		edge.To("automations", Automation.Type),
		edge.To("automation_versions", AutomationVersion.Type),
		edge.To("automation_runs", AutomationRun.Type),
		edge.To("webhook_endpoints", WebhookEndpoint.Type),
		edge.To("suppressions", Suppression.Type),
		edge.To("unsubscribes", Unsubscribe.Type),
		edge.To("confirmations", Confirmation.Type),
		edge.To("outbound_messages", OutboundMessage.Type),
		edge.To("memberships", Membership.Type),
		edge.To("invitations", Invitation.Type),
		edge.To("audit_entries", AuditEntry.Type),
		edge.To("erasures", Erasure.Type),
		edge.To("usages", Usage.Type),
	}
}

// workspaceTable adds the length of the public id to the enum checks: ent stores it as an
// unbounded varchar, and the suspension columns that are written and cleared together
// (internal/suspension): who and why exist exactly while suspended_at does, and an id names
// only a Staff member.
func workspaceTable() entsql.Annotation {
	a := TableWithEnumChecks("workspaces", Workspace{}.Fields())
	a.Checks["workspaces_suspension_check"] = "(suspended_at IS NULL) = (suspended_by_kind IS NULL)" +
		" AND (suspended_at IS NULL) = (suspension_reason IS NULL)" +
		" AND COALESCE(suspended_by_kind = 'staff', false) = (suspended_by_id IS NOT NULL)"
	a.Checks["workspaces_public_id_max_length_check"] = "char_length(public_id) <= 8"
	a.Checks["workspaces_public_id_min_length_check"] = "char_length(public_id) >= 8"
	return a
}
