package schema

import (
	"fmt"
	"strings"

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

// Annotations adds to the enum checks that an Integration's channel is its provider's: a provider
// sends on one Channel (providers.Provider.Channel), and a row that paired it with another would be
// picked as that Channel's default and routed to the wrong sender.
func (Integration) Annotations() []schema.Annotation {
	a := TableWithEnumChecks("integrations", Integration{}.Fields())
	a.Checks["integrations_provider_channel_check"] = providerChannelCheck()
	return []schema.Annotation{
		a,
		Audited{Action: "integration", NameField: NameColumn},
	}
}

func (Integration) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "integrations"}, DiscardMixin{}, RevisionMixin{}}
}

func (Integration) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.String(NameColumn).
			NotEmpty(),
		// The provider's Channel (providers.Provider.Channel), stored for the per-Channel indexes
		// and checked against the provider; no default, so a writer cannot pair a provider with a
		// Channel it does not send on by leaving it out.
		field.Enum("channel").
			GoType(channels.Channel("")),
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
			Annotations(Sensitive{}, UniqueOverAll{
				Reason: "a hook's address names one Integration for ever: a key a discarded Integration held is never served again",
			}),
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
	}
}

func (Integration) Edges() []ent.Edge {
	return []ent.Edge{
		// Purging an Integration (Workspace erasure) deletes its Provider limit state with it.
		edge.To("provider_limiter", ProviderLimiter.Type).
			Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
		// What the provider last said about it (ADR 0168) dies with it.
		edge.To("provider_state", IntegrationProviderState.Type).
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
		// The envelopes a Sending domain's check read through it (ADR 0184) die with it.
		edge.To("envelopes", SendingDomainEnvelope.Type).
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
		// At most one Test inbox per Workspace (ADR 0058); an archived one frees the place,
		// so restoring it while another is kept trips this index.
		index.Fields("workspace_id", "provider").
			StorageKey("integrations_one_inbox_per_workspace").
			Annotations(entsql.IndexWhere("provider = 'inbox' AND discarded_at IS NULL")).
			Unique(),
	}
}

// providerChannelCheck is one arm per Channel, naming the providers that send on it.
func providerChannelCheck() string {
	var arms []string
	for _, ch := range channels.Channel("").Values() {
		ps := providers.OnChannel(channels.Channel(ch))
		if len(ps) == 0 {
			continue
		}
		quoted := make([]string, len(ps))
		for i, p := range ps {
			quoted[i] = "'" + string(p) + "'"
		}
		arms = append(arms, fmt.Sprintf("(channel = '%s' AND provider IN (%s))", ch, strings.Join(quoted, ", ")))
	}
	return strings.Join(arms, " OR ")
}
