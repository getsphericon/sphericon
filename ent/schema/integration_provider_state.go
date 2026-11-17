package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/internal/kernel/providers"
)

// IntegrationProviderState is what the provider last said about one Integration (ADR 0168): its
// quota, the account's standing and a refusal of its credentials. Jobs and sends write it, so it
// lives beside the Integration and moves no revision of it; one row per Integration, made with it.
type IntegrationProviderState struct {
	ent.Schema
}

func (IntegrationProviderState) Annotations() []schema.Annotation {
	return []schema.Annotation{
		TableWithEnumChecks("integration_provider_state", IntegrationProviderState{}.Fields()),
	}
}

func (IntegrationProviderState) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "integration_provider_states"}}
}

func (IntegrationProviderState) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("integration_id").
			Unique().
			Immutable(),
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

func (IntegrationProviderState) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("integration", Integration.Type).
			Ref("provider_state").
			Field("integration_id").
			Unique().
			Required().
			Immutable(),
	}
}

func (IntegrationProviderState) Indexes() []ent.Index {
	return []ent.Index{
		// Serves both foreign keys: the Workspace's and the Integration's composite one.
		index.Fields("workspace_id", "integration_id"),
	}
}
