package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Integration is a workspace-scoped connection to an external sending provider.
// It is channel-agnostic on purpose: "email" today (smtp/ses), "sms" and others
// plug in the same way later by adding enum values + a catalog descriptor, with
// no schema reshape. Provider-specific credentials live encrypted in
// config_encrypted (the cleartext JSON shape is owned by the messaging catalog).
type Integration struct {
	ent.Schema
}

func (Integration) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "integrations"},
		Audited{Action: "integration", NameField: "name"},
	}
}

func (Integration) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "integrations"}}
}

func (Integration) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.String("name").
			NotEmpty(),
		// Channel reserves "sms" now so the column/type is ready before any SMS
		// provider exists.
		field.Enum("channel").
			Values("email", "sms").
			Default("email"),
		field.Enum("provider").
			Values("smtp", "ses"),
		// Encrypted JSON blob produced by internal/secrets.Cipher.
		field.String("config_encrypted").
			Sensitive().
			Annotations(Sensitive{}),
		// The Preset this Integration's provider callbacks are read with (ADR 0037); nil
		// when the Integration receives none. An SES Integration with no Preset is read by
		// the SES adapter until the `ses` Preset replaces it.
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
			Default(true),
		field.Bool("is_default").
			Default(false),
		// Send rate limit (ADR 0023): the most messages that may leave this Integration
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
		// When the quota was last looked up, successfully or not.
		field.Time("provider_quota_checked_at").
			Optional().
			Nillable(),
		// The last lookup failed (no ses:GetSendQuota permission, an SES-compatible
		// service); the UI warns and a later success clears it.
		field.Bool("provider_quota_unavailable").
			Default(false),
	}
}

func (Integration) Edges() []ent.Edge {
	return []ent.Edge{
		// Deleting an Integration deletes its Send rate limiter state with it.
		edge.To("send_limiter", SendLimiter.Type).
			Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (Integration) Indexes() []ent.Index {
	return []ent.Index{
		// At most one default provider per (workspace, channel). Partial index so
		// only is_default rows are constrained.
		index.Fields("workspace_id", "channel").
			Annotations(entsql.IndexWhere("is_default")).
			Unique(),
	}
}
