// The Webhook endpoint models /site and /api show and take the same way.

namespace Sphericon;

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
  /** The webhook's `revision` the edit was made on (ADR 0119): the one it was read with */
  revision: Revision;

  /** Destination URL (absolute http or https) */
  url?: string;

  /** Event names to receive; empty means all events */
  eventTypes?: string[];

  /** Whether the endpoint receives deliveries */
  enabled?: boolean;
}

/** Request body for sending a sample event to a webhook endpoint */
model TestWebhookInput {
  /** The event name the sample carries, for example `contact.created` */
  eventType: string;
}

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

/**
 * Why an endpoint is off. A closed set: `owner` (a member turned it off), `gone` (the
 * receiver answered 410 Gone) and `persistent_failure` (deliveries failed for longer
 * than the window). Absent while the endpoint is enabled.
 */
enum WebhookDisabledReason {
  owner: "owner",
  gone: "gone",
  persistent_failure: "persistent_failure",
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

/** What the receiver answered to a test delivery. The endpoint's failure state is not touched. */
model WebhookTestResult {
  /** Whether the receiver answered 2xx */
  delivered: boolean;

  /** HTTP status the receiver answered, when it answered */
  status?: int32;

  /** Why the delivery failed; absent when delivered */
  kind?: WebhookErrorKind;
}
