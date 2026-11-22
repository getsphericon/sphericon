package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
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
		webhookEndpointTable(),
		Audited{Action: "webhook_endpoint", NameField: "url", NameIsURL: true},
	}
}

func (WebhookEndpoint) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "webhook_endpoints"}, RevisionMixin{}}
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
		// HMAC signing secret, encrypted at rest via internal/kernel/secrets.Cipher.
		field.String("secret_encrypted").
			Sensitive().
			Annotations(Sensitive{}),
		// Event names this endpoint subscribes to; empty/nil means all events.
		field.Strings("event_types").
			Optional(),
		// When the endpoint was turned off (nil = enabled), with disabled_reason saying why.
		// Written by an owner or by the delivery worker, so a write is a compare-and-swap
		// on nil.
		field.Time("disabled_at").
			Annotations(Machine()).
			Optional().
			Nillable(),
		// An explicit owner enable starts a new health generation. A delivery loaded before
		// that reset can finish its HTTP request, but its result cannot change this run.
		// Null is the initial generation, before any enable.
		field.Int64("health_generation").
			Optional().
			Nillable().
			NonNegative().
			// Moved by the enable it follows from, which the entry records.
			Annotations(Derived{}),
		// Why the endpoint is off (ADR 0039): an owner turned it off, the receiver answered 410,
		// or deliveries failed for longer than the window. Set exactly when disabled_at is.
		field.Enum("disabled_reason").
			Values("owner", "gone", "persistent_failure").
			Optional().
			Nillable(),
	}
}

func (WebhookEndpoint) Edges() []ent.Edge {
	return []ent.Edge{
		// Its failure run (ADR 0168) dies with it.
		edge.To("health", WebhookEndpointHealth.Type).
			Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (WebhookEndpoint) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "disabled_at"),
	}
}

// webhookEndpointTable adds to the enum checks that an endpoint is disabled with a reason and
// enabled without one.
func webhookEndpointTable() entsql.Annotation {
	a := TableWithEnumChecks("webhook_endpoints", WebhookEndpoint{}.Fields())
	a.Checks["webhook_endpoints_disabled_check"] = "(disabled_at IS NULL) = (disabled_reason IS NULL)"
	return a
}
