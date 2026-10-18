using TypeSpec.Http;
using TypeSpec.Rest;

namespace Sphericon;

/**
 * Why a delivery to a webhook endpoint failed. A closed set: `timeout` (no answer in
 * time), `blocked_address` (the URL names an address this instance does not permit),
 * `http_status` (a non-2xx answer, see `status`) and `network` (any other transport
 * failure).
 */
enum WebhookErrorKind {
  timeout: "timeout",
  blocked_address: "blocked_address",
  http_status: "http_status",
  network: "network",
}

/** The failure state of an endpoint that is currently failing; absent while it is healthy. */
model WebhookFailure {
  /** Deliveries that failed in a row */
  consecutiveFailures: int32;

  /** When the current run of failures began */
  since: Timestamp;

  /** When the latest delivery failed */
  lastFailureAt: Timestamp;

  /** Kind of the latest failure */
  kind: WebhookErrorKind;

  /** HTTP status of the latest failure; present only for kind `http_status` */
  status?: int32;
}

/**
 * Outbound webhook endpoint. The HMAC signing secret is deliberately absent:
 * it is never returned through the external API (view it in the app).
 */
@resource("webhooks")
@segment("webhooks")
model WebhookResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** Destination URL events are POSTed to */
  url: string;

  /** Event names this endpoint receives; empty means all events */
  eventTypes: string[];

  /** Whether the endpoint receives deliveries */
  enabled: boolean;

  /** Present while deliveries are failing; cleared by the next 2xx */
  failure?: WebhookFailure;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Request body for creating a webhook endpoint */
model CreateWebhookInput {
  /** Destination URL (absolute http or https) */
  url: string;

  /** Event names to receive; empty or absent means all events */
  eventTypes?: string[];

  /** Whether the endpoint receives deliveries (default true) */
  enabled?: boolean;
}

/** Request body for updating a webhook endpoint */
model UpdateWebhookInput {
  /** Destination URL (absolute http or https) */
  url?: string;

  /** Event names to receive; empty means all events */
  eventTypes?: string[];

  /** Whether the endpoint receives deliveries */
  enabled?: boolean;
}

@tag("Webhooks")
interface Webhooks
  extends ExternalCrud<
      WebhookResource,
      CreateWebhookInput,
      UpdateWebhookInput
    > {}
