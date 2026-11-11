package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
)

// Integration is a workspace-scoped connection to an external sending provider.
// It is channel-agnostic on purpose: "email" (smtp/ses/inbox), "telegram" (a bot) and "sms" (Twilio);
// another Channel plugs in the same way by adding enum values + a catalog descriptor, with
// no schema reshape. Provider-specific credentials live encrypted in
// config_encrypted (the cleartext JSON shape is owned by the messaging catalog).
type Integration struct {
	ent.Schema
}

func (Integration) Annotations() []schema.Annotation {
	return []schema.Annotation{
		TableWithEnumChecks("integrations", Integration{}.Fields()),
		Audited{Action: "integration", NameField: nameField},
	}
}

func (Integration) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "integrations"}, DiscardMixin{}}
}

func (Integration) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.String(nameField).
			NotEmpty(),
		field.Enum("channel").
			GoType(channels.Channel("")).
			Default("email"),
		field.Enum("provider").
			GoType(providers.Provider("")),
		// Encrypted JSON blob produced by internal/kernel/secrets.Cipher.
		field.String("config_encrypted").
			Sensitive().
			Annotations(Sensitive{}),
		// The Preset this Integration's provider callbacks are read with (ADR 0037); nil
		// when the Integration names none. An SES Integration with no Preset is read with the
		// `ses` Preset: the hook implies it, and creating one stores it.
		field.String("preset").
			Optional().
			Nillable().
			NotEmpty(),
		// Encrypted JSON of the Preset's params (secrets included; a secret that is
		// rotating holds the current and the retiring value). Never returned by a read.
		field.String("params_encrypted").
			Optional().
			Sensitive().
			Annotations(Sensitive{}),
		// The secret key of this Integration's hook, POST /hooks/{ingest_key}: it names the
		// Integration and so its Workspace. It is an address, not the authentication (the
		// Preset's signature or secret is) and regenerating it revokes the old URL.
		field.String("ingest_key").
			NotEmpty().
			Unique().
			Annotations(Sensitive{}),
		field.Bool("enabled").
			Annotations(Attribute(reasonSetting)).
			Default(true),
		field.Bool("is_default").
			Annotations(Attribute(reasonSetting), GivenUpOnDiscard{}).
			Default(false),
		// Provider limit (ADR 0023): the most messages that may leave this Integration
		// per second and per rolling 24 hours. Nil means no limit of that kind.
		field.Int("max_per_second").
			Optional().
			Nillable().
			Positive(),
		field.Int("max_per_day").
			Optional().
			Nillable().
			Positive(),
		// Provider-reported quota (ADR 0023): what SES GetSendQuota last said, folded into
		// the effective ceiling by the minimum rule. Nil means the provider reported none.
		field.Int("provider_max_per_second").
			Optional().
			Nillable(),
		field.Int("provider_max_per_day").
			Optional().
			Nillable(),
		// The account's standing (ADR 0067) is, like provider_max_*, a reading of the provider
		// that each successful read overwrites whole, not a state the platform moves: nobody
		// asks when it changed, and provider_quota_checked_at says when it was read. A save of
		// new credentials clears it, as it may name another account.
		//
		// What the provider last said it is doing about the account (SES EnforcementStatus);
		// nil is nothing. `shutdown` holds every send through the Integration until a read
		// finds the account sending again; `probation` is a warning.
		field.Enum("provider_enforcement").
			GoType(providers.Enforcement("")).
			Optional().
			Nillable(),
		// Whether the provider last said the account is in its sandbox, delivering only to
		// verified addresses (SES without production access); a warning, not a hold.
		field.Bool("provider_sandbox").
			Annotations(Attribute(reasonLatest)).
			Default(false),
		// When a send found the provider refusing the Integration's credentials (Telegram's 401):
		// the Integration is held (provider_refused) until it is saved again with credentials the
		// provider accepts, which clears it. Nil is not refused. Unlike provider_enforcement it is
		// found at send time, not read from the provider.
		field.Time("provider_refused_at").
			Annotations(Machine()).
			Optional().
			Nillable(),
		// When the quota was last looked up, successfully or not.
		field.Time("provider_quota_checked_at").
			Annotations(Attribute(reasonLatest)).
			Optional().
			Nillable(),
		// When the lookup first failed (no ses:GetSendQuota permission, an SES-compatible
		// service) in the failing run it is still in; the UI warns while it is set and a
		// later success clears it. A further failure keeps the first moment.
		field.Time("provider_quota_failed_at").
			Annotations(Attribute(reasonFoundRun)).
			Optional().
			Nillable(),
	}
}

func (Integration) Edges() []ent.Edge {
	return []ent.Edge{
		// Purging an Integration (Workspace erasure) deletes its Provider limit state with it.
		edge.To("provider_limiter", ProviderLimiter.Type).
			Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
		// What a Test inbox kept (ADR 0058) dies with the Integration; only Workspace deletion
		// removes an Integration, since an Integration is archived, never deleted (ADR 0049).
		edge.To("captured_messages", CapturedMessage.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		// The chats the bot reached and the Start tokens issued for it die with it likewise.
		edge.To("bindings", Binding.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("start_tokens", StartToken.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (Integration) Indexes() []ent.Index {
	return []ent.Index{
		// At most one default provider per (workspace, channel). Partial index so
		// only kept is_default rows are constrained (an archived Integration holds no default).
		index.Fields("workspace_id", "channel").
			Annotations(entsql.IndexWhere("is_default AND discarded_at IS NULL")).
			Unique(),
		index.Fields("workspace_id"),
		// At most one Test inbox per Workspace (ADR 0058); an archived one frees the place,
		// so restoring it while another is kept trips this index.
		index.Fields("workspace_id", "provider").
			StorageKey("integrations_one_inbox_per_workspace").
			Annotations(entsql.IndexWhere("provider = 'inbox' AND discarded_at IS NULL")).
			Unique(),
	}
}
