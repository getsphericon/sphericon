using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

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

/**
 * Why an endpoint turned itself off. A closed set: `gone` (the receiver answered 410
 * Gone) and `persistent_failure` (deliveries failed for longer than the window).
 * Absent for an endpoint an owner disabled, and while the endpoint is enabled.
 */
enum WebhookDisabledReason {
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

  /** Why the endpoint turned itself off; absent when an owner disabled it or it is enabled */
  disabledReason?: WebhookDisabledReason;

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

/** Request body for sending a sample event to a webhook endpoint */
model TestWebhookInput {
  /** The event name the sample carries, for example `contact.created` */
  eventType: string;
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

@tag("Webhooks")
interface Webhooks
  extends ExternalCrud<
      WebhookResource,
      CreateWebhookInput,
      UpdateWebhookInput
    > {
  /** Replace the endpoint's signing secret; the old one stops verifying at once. The new secret is never returned here: read it in the app. Needs the webhooks:regenerate scope. */
  @action("regenerate-secret")
  @autoRoute
  regenerateSecret(...ResourceParameters<WebhookResource>):
    | WebhookResource
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  /** Send a sample event of the chosen type, signed as a real delivery, and return the receiver's answer. Does not change the endpoint's failure state. Needs the webhooks:test scope. */
  @action("test")
  @autoRoute
  test(...ResourceParameters<WebhookResource>, @body body: TestWebhookInput):
    | WebhookTestResult
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;
}
