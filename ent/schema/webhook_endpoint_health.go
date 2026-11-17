package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/internal/infra/webhook"
)

// WebhookEndpointHealth is the current failure run of one webhook endpoint (ADR 0039, ADR 0168),
// written by the delivery worker with no actor, so it is not audited and moves no revision of the
// endpoint. One row per endpoint, made with it; all nil while the endpoint is healthy, and a 2xx
// clears them.
type WebhookEndpointHealth struct {
	ent.Schema
}

func (WebhookEndpointHealth) Annotations() []schema.Annotation {
	return []schema.Annotation{
		TableWithEnumChecks("webhook_endpoint_health", WebhookEndpointHealth{}.Fields()),
	}
}

func (WebhookEndpointHealth) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "webhook_endpoint_healths"}}
}

func (WebhookEndpointHealth) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("webhook_endpoint_id").
			Unique().
			Immutable(),
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
			GoType(webhook.ErrorKind("")).
			Optional().
			Nillable(),
		field.Int("last_error_status").
			Optional().
			Nillable(),
		field.Time("last_failure_at").
			Annotations(Attribute(reasonLatest)).
			Optional().
			Nillable(),
	}
}

func (WebhookEndpointHealth) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("webhook_endpoint", WebhookEndpoint.Type).
			Ref("health").
			Field("webhook_endpoint_id").
			Unique().
			Required().
			Immutable(),
	}
}

func (WebhookEndpointHealth) Indexes() []ent.Index {
	return []ent.Index{
		// Serves both foreign keys: the Workspace's and the endpoint's composite one.
		index.Fields("workspace_id", "webhook_endpoint_id"),
	}
}
