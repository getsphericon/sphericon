using TypeSpec.Http;
using TypeSpec.Rest;

namespace Sphericon;

/**
 * Why a delivery to a webhook endpoint failed. A closed set: `timeout` (no answer in
 * time), `blocked_address` (the URL names an address this instance does not permit),
 * `http_status` (a non-2xx answer, see `status`) and `network` (any other transport
 * failure).
 */
enum SiteWebhookErrorKind {
  timeout: "timeout",
  blocked_address: "blocked_address",
  http_status: "http_status",
  network: "network",
}

/** The failure state of an endpoint that is currently failing; absent while it is healthy. */
model SiteWebhookFailure {
  /** Deliveries that failed in a row */
  consecutiveFailures: int32;

  /** When the current run of failures began */
  since: Timestamp;

  /** When the latest delivery failed */
  lastFailureAt: Timestamp;

  /** Kind of the latest failure */
  kind: SiteWebhookErrorKind;

  /** HTTP status of the latest failure; present only for kind `http_status` */
  status?: int32;
}

/** Webhook endpoint resource used by the site UI */
@resource("webhooks")
@segment("webhooks")
@parentResource(SiteWorkspaceResource)
model SiteWebhookEndpointResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** Destination URL events are POSTed to */
  url: string;

  /** HMAC signing secret (receivers verify the webhook-id, webhook-timestamp and webhook-signature headers, the Standard Webhooks scheme) */
  secret: string;

  /** Event names this endpoint receives; empty means all events */
  eventTypes: string[];

  /** Whether the endpoint receives deliveries */
  enabled: boolean;

  /** Present while deliveries are failing; cleared by the next 2xx */
  failure?: SiteWebhookFailure;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Site request body for creating a webhook endpoint */
model SiteCreateWebhookEndpointInput {
  url: string;
  eventTypes?: string[];
  enabled?: boolean;
}

/** Site request body for updating a webhook endpoint */
model SiteUpdateWebhookEndpointInput {
  url?: string;
  eventTypes?: string[];
  enabled?: boolean;
}

@tag("Site Webhooks")
interface SiteWebhooks
  extends WorkspaceCrud<
      SiteWebhookEndpointResource,
      SiteCreateWebhookEndpointInput,
      SiteUpdateWebhookEndpointInput
    > {}
