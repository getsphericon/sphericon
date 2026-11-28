// Package schema holds the ent schemas: the tables, columns, constraints, indexes and mixins of the
// data model, from which ent, the scoped client and the migrations are generated.
package schema

import (
	"strings"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"

	"github.com/getsphericon/sphericon/internal/kernel/domainname"
)

type Workspace struct {
	ent.Schema
}

func (Workspace) Annotations() []schema.Annotation {
	return []schema.Annotation{
		workspaceTable(),
		// Its settings are edited through the scope's own tenant edit (Scoped.EditWorkspaceAt), which
		// records them as workspace.update.
		Audited{Action: "workspace", NameField: NameColumn},
	}
}

func (Workspace) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, DiscardMixin{Restorable: true}, RevisionMixin{}}
}

func (Workspace) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// Free text, never empty while the Workspace lives (the handlers refuse an empty name);
		// the erasure job blanks it on the tombstone (ADR 0043), so the schema allows it.
		field.String(NameColumn),
		// The Workspace id (ADR 0043): 8 random base62 characters, the Workspace's one public
		// address. Assigned at creation, never changed, unique; internal/kernel/workspaceid makes it.
		field.String("public_id").
			MinLen(8).
			MaxLen(8).
			Unique().
			Immutable().
			Annotations(UniqueOverAll{Reason: "the Workspace's one public address, which a discarded Workspace keeps to be restored"}),
		field.String("collect_key").
			NotEmpty().
			Unique().
			Sensitive().
			Annotations(Sensitive{}, UniqueOverAll{Reason: "a collect key names one Workspace for ever, and a discarded Workspace keeps it to be restored"}),
		// Confirmed opt-in policy (ADR 0013). When true, marketing sends require a
		// Confirmation for the destination (double opt-in); when false (the default),
		// the subtractive single-opt-in model applies unchanged. Only a person turns it on, in
		// the console, and only while the confirmation email is set up: enabling it records a
		// grandfathered Confirmation for every mailable address rather than muting the existing
		// list.
		field.Bool("require_confirmed_opt_in").
			Annotations(Attribute(reasonSetting)).
			Default(false),
		// The confirmation email's settings (ADR 0013): the From, an address on a verified Sending
		// domain (checked when it is saved), which marks the email set up, its name, the subject
		// and the introductory paragraph. The rest of the email is fixed by sphericon. A NULL name,
		// subject or intro reads as the Instance's default naming the Workspace, so a rename
		// carries into it.
		field.String("confirmation_from_email").
			Optional().
			Nillable().
			NotEmpty(),
		field.String("confirmation_from_name").
			Optional().
			Nillable().
			NotEmpty(),
		field.String("confirmation_subject").
			Optional().
			Nillable().
			NotEmpty(),
		field.String("confirmation_intro").
			Optional().
			Nillable().
			NotEmpty(),
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
		// The Workspace time zone (ADR 0057): the IANA name the analytics reports count their days
		// in. NULL reads as UTC. The name is checked against the zone database by
		// internal/accounts, which the database cannot do.
		field.String("timezone").
			Optional().
			Nillable().
			NotEmpty(),
		// The Engagement tracking default (ADR 0120): whether a Destination with no Engagement
		// tracking consent recorded is tracked. `track` until an Owner or Admin changes it; the core
		// picks no default by law. The send path reads it at render and the recorder at every hit.
		// A turn to `dont_track` queues the withdrawal by the default, in the edit's transaction.
		field.Enum("engagement_tracking_default").
			Values("track", "dont_track").
			Default("track").
			Annotations(Attribute("a setting its owner edits, on the Workspace's revision; the withdrawal a turn to dont_track " +
				"queues in the same transaction is idempotent and bound to the moment of the turn, so a repeated or reverted turn needs no compare-and-swap")),
		// The Reporting currency (ADR 0064): the ISO 4217 code every revenue figure is in. NULL until
		// an Owner or Admin chooses one; internal/accounts checks the code against the registry.
		field.String("reporting_currency").
			Optional().
			Nillable().
			NotEmpty(),
		// The Attribution window (ADR 0064) in whole days; its bounds are declared in the contract.
		field.Int("attribution_window_days").
			Default(5).
			Positive(),
		// The Send cap (ADR 0067): the most messages the Workspace may send per rolling 24 hours,
		// across all its Integrations, as a Staff member set it. NULL: no Send cap. It applies only
		// on an Instance that has Send caps (the send path is built with them); elsewhere a stored
		// value is ignored. Its bounds are declared in the contract.
		field.Int("send_cap").
			Optional().
			Nillable().
			Positive(),
		// Domain join (ADR 0051): whether Users who prove an address on one of the Workspace's
		// verified Join domains become its members. Off until an Owner switches it on.
		field.Bool("domain_join_enabled").
			Annotations(Attribute(reasonSetting)).
			Default(false),
		// Link identity tokens (ADR 0160): whether the click of a tracked link adds the recipient's
		// token to a destination on one of link_identity_hosts. Off until an Owner or Admin turns it
		// on, which needs a host listed (workspaces_link_identity_check).
		field.Bool("link_identity_enabled").
			Annotations(Attribute(reasonSetting)).
			Default(false),
		// The hostnames of the Workspace's own sites, where its tracker runs: each normalized as a
		// Sending domain is (domainname), matched exactly, so a subdomain is listed on its own.
		field.Strings("link_identity_hosts").
			Default([]string{}).
			Annotations(entsql.Default("[]")),
		// Two-factor requirement (ADR 0020): when an Owner or Admin switched it on;
		// NULL while off. Each member's grace ends accounts.GracePeriod after the
		// later of this and their Membership's creation.
		field.Time("second_factor_required_at").
			Annotations(Attribute(reasonSetting)).
			Optional().
			Nillable(),
		// Workspace deletion (ADR 0043): discarded_at (DiscardMixin) starts the 30-day window, in
		// which the Workspace is frozen, hidden and restorable; erased_at is set when the erasure
		// job has removed its data and left the row as a tombstone. Both NULL for a live Workspace.
		field.Time("erased_at").
			Annotations(Machine()).
			Optional().
			Nillable(),
		field.Time("suspended_at").
			Annotations(Machine()).
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
			Nillable().
			Annotations(ScopedRef{
				Unchecked: true,
				Reason:    "snapshot of the Staff member who suspended: the Staff member table is not part of the Workspace's data, and the id must survive the member's removal",
			}),
		field.String("suspension_reason").
			Optional().
			Nillable(),
		// When a Staff member last lifted a suspension (ADR 0026): the abuse detector counts
		// only Events after it, so a cleared sender is not frozen again for old data.
		field.Time("cleared_at").
			Annotations(Attribute(reasonHistory)).
			Optional().
			Nillable(),
		// Set and cleared only by a Staff member: an exempt Workspace is neither warned nor
		// frozen by the abuse detector (it can still be suspended by hand).
		field.Bool("abuse_exempt").
			Annotations(Attribute(reasonSetting)).
			Default(false),
	}
}

func (Workspace) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("contacts", Contact.Type),
		edge.To("custom_fields", CustomField.Type),
		edge.To("tags", Tag.Type),
		edge.To("contact_tags", ContactTag.Type),
		edge.To("contact_dates", ContactDate.Type),
		edge.To("segments", Segment.Type),
		edge.To("events", Event.Type),
		edge.To("event_journal_entries", EventJournalEntry.Type).Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.To("orders", Order.Type),
		edge.To("order_movements", OrderMovement.Type),
		edge.To("held_order_events", HeldOrderEvent.Type),
		edge.To("visitors", Visitor.Type),
		edge.To("bindings", Binding.Type),
		edge.To("start_tokens", StartToken.Type),
		edge.To("download_links", DownloadLink.Type),
		edge.To("api_tokens", ApiToken.Type),
		edge.To("oauth_codes", OAuthCode.Type),
		edge.To("integrations", Integration.Type),
		edge.To("sending_domains", SendingDomain.Type),
		edge.To("join_domains", JoinDomain.Type),
		edge.To("tracking_domains", TrackingDomain.Type),
		edge.To("provider_limiters", ProviderLimiter.Type),
		edge.To("send_cap_limiters", SendCapLimiter.Type),
		edge.To("holds", Hold.Type),
		edge.To("frequency_caps", FrequencyCap.Type),
		edge.To("frequency_counts", FrequencyCount.Type),
		edge.To("contact_imports", ContactImport.Type),
		edge.To("contact_import_chunks", ContactImportChunk.Type),
		edge.To("contact_import_report_rows", ContactImportReportRow.Type),
		edge.To("sending_windows", SendingWindow.Type),
		edge.To("broadcasts", Broadcast.Type),
		edge.To("broadcast_recipients", BroadcastRecipient.Type),
		edge.To("broadcast_figures", BroadcastFigure.Type),
		edge.To("automation_figures", AutomationFigure.Type),
		edge.To("automation_skipped_entries", AutomationSkippedEntry.Type),
		edge.To("automation_occurrence_cursors", AutomationOccurrenceCursor.Type),
		edge.To("email_templates", EmailTemplate.Type),
		edge.To("brand_kits", BrandKit.Type),
		edge.To("identity_secrets", IdentitySecret.Type),
		edge.To("automations", Automation.Type),
		edge.To("automation_versions", AutomationVersion.Type),
		edge.To("automation_runs", AutomationRun.Type),
		edge.To("automation_step_records", AutomationStepRecord.Type),
		edge.To("rules", Rule.Type),
		edge.To("rule_versions", RuleVersion.Type),
		edge.To("rule_version_triggers", RuleVersionTrigger.Type),
		edge.To("rule_firings", RuleFiring.Type),
		edge.To("webhook_endpoints", WebhookEndpoint.Type),
		edge.To("suppressions", Suppression.Type),
		edge.To("unsubscribes", Unsubscribe.Type),
		edge.To("confirmations", Confirmation.Type),
		edge.To("consent_stamps", ConsentStamp.Type),
		edge.To("engagement_tracking_consents", EngagementTrackingConsent.Type),
		edge.To("outbound_messages", OutboundMessage.Type),
		edge.To("tracked_links", TrackedLink.Type),
		edge.To("captured_messages", CapturedMessage.Type),
		edge.To("memberships", Membership.Type),
		edge.To("invitations", Invitation.Type),
		edge.To("audit_entries", AuditEntry.Type),
		edge.To("erasures", Erasure.Type),
		edge.To("usages", Usage.Type),
		edge.To("integration_provider_states", IntegrationProviderState.Type),
		edge.To("sending_domain_envelopes", SendingDomainEnvelope.Type),
		edge.To("webhook_endpoint_healths", WebhookEndpointHealth.Type),
		edge.To("standing", WorkspaceStanding.Type).Unique(),
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
	// Each listed host is in the form domainname writes, and the switch is on only with a host listed.
	a.Checks["workspaces_link_identity_hosts_check"] = "jsonb_typeof(link_identity_hosts) = 'array' AND NOT (link_identity_hosts @? " +
		"'$[*] ? (@.type() != \"string\" || !(@ like_regex \"" + strings.ReplaceAll(domainname.Pattern, `\`, `\\`) + "\"))')"
	a.Checks["workspaces_link_identity_check"] = "NOT link_identity_enabled OR jsonb_array_length(link_identity_hosts) > 0"
	currency, currencyExpr := CurrencyCheck("workspaces", "reporting_currency")
	a.Checks[currency] = currencyExpr
	return a
}
