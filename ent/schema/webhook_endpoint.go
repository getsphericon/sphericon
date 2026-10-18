package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WebhookEndpoint is a workspace-scoped HTTP destination that receives domain
// events. The signing secret is stored encrypted (like integration creds); the
// "webhooks" bus subscriber fans matching events out to a river delivery job.
type WebhookEndpoint struct {
	ent.Schema
}

func (WebhookEndpoint) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "webhook_endpoints"},
		Audited{Action: "webhook_endpoint", NameField: "url", NameIsURL: true},
	}
}

func (WebhookEndpoint) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "webhook_endpoints"}}
}

func (WebhookEndpoint) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The URL may carry a secret (a token in the path or query), so a diff records
		// only that it changed, and the name snapshot drops the query and userinfo.
		field.String("url").
			NotEmpty().
			Annotations(Sensitive{}),
		// HMAC signing secret, encrypted at rest via internal/secrets.Cipher.
		field.String("secret_encrypted").
			Sensitive().
			Annotations(Sensitive{}),
		// Event names this endpoint subscribes to; empty/nil means all events.
		field.Strings("event_types").
			Optional(),
		field.Bool("enabled").
			Default(true),
		// Failure state (ADR 0039), written by the delivery worker with no actor so it
		// is not audited. All nil while the endpoint is healthy; a 2xx clears them.
		// Failed deliveries in a row.
		field.Int("consecutive_failures").
			Optional().
			Nillable(),
		// When the current run of failures began.
		field.Time("failing_since").
			Optional().
			Nillable(),
		// Closed kind of the latest failure; http_status carries last_error_status.
		field.Enum("last_error_kind").
			Values("timeout", "blocked_address", "http_status", "network").
			Optional().
			Nillable(),
		field.Int("last_error_status").
			Optional().
			Nillable(),
		field.Time("last_failure_at").
			Optional().
			Nillable(),
	}
}

func (WebhookEndpoint) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id"),
		index.Fields("workspace_id", "enabled"),
	}
}
