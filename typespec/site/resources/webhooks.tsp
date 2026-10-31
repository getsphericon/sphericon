using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

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

/**
 * Why an endpoint is off. A closed set: `owner` (a member turned it off), `gone` (the
 * receiver answered 410 Gone) and `persistent_failure` (deliveries failed for longer
 * than the window). Absent while the endpoint is enabled.
 */
enum SiteWebhookDisabledReason {
  owner: "owner",
  gone: "gone",
  persistent_failure: "persistent_failure",
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

  /** Whether the endpoint receives deliveries: true while it has no `disabledAt` */
  enabled: boolean;

  /** When the endpoint was turned off; absent while it is enabled */
  disabledAt?: Timestamp;

  /** Why the endpoint is off; present exactly when `disabledAt` is */
  disabledReason?: SiteWebhookDisabledReason;

  /** Present while deliveries are failing; cleared by the next 2xx */
  failure?: SiteWebhookFailure;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** What the receiver answered to a test delivery. The endpoint's failure state is not touched. */
model SiteWebhookTestResult {
  /** Whether the receiver answered 2xx */
  delivered: boolean;

  /** HTTP status the receiver answered, when it answered */
  status?: int32;

  /** Why the delivery failed; absent when delivered */
  kind?: SiteWebhookErrorKind;
}

/**
 * Webhook endpoints are managed by owners and admins only: the resource carries the
 * signing secret, so every operation, reads included, is refused to a member with 403.
 */
@tag("Site Webhooks")
interface SiteWebhooks {
  /** List webhook endpoints. Owners and admins only. */
  @listsResource(SiteWebhookEndpointResource)
  @get
  @autoRoute
  @requiresPermissions(#[Permission.webhooksManage])
  list(
    ...ResourceCollectionParameters<SiteWebhookEndpointResource>,
    ...PageQuery,
  ):
    | Page<SiteWebhookEndpointResource>
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Create a webhook endpoint. Owners and admins only. */
  @createsResource(SiteWebhookEndpointResource)
  @post
  @autoRoute
  @requiresPermissions(#[Permission.webhooksManage])
  create(
    ...ResourceCollectionParameters<SiteWebhookEndpointResource>,
    @body body: CreateWebhookInput,
  ):
    | {
        @statusCode statusCode: 201;
        @body body: SiteWebhookEndpointResource;
      }
    | ForbiddenProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Get a webhook endpoint by ID. Owners and admins only. */
  @readsResource(SiteWebhookEndpointResource)
  @get
  @autoRoute
  @requiresPermissions(#[Permission.webhooksManage])
  get(...ResourceParameters<SiteWebhookEndpointResource>):
    | SiteWebhookEndpointResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem;

  /** Update a webhook endpoint. Owners and admins only. */
  @updatesResource(SiteWebhookEndpointResource)
  @put
  @autoRoute
  @requiresPermissions(#[Permission.webhooksManage])
  update(
    ...ResourceParameters<SiteWebhookEndpointResource>,
    @body body: UpdateWebhookInput,
  ):
    | SiteWebhookEndpointResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Delete a webhook endpoint. Owners and admins only. */
  @deletesResource(SiteWebhookEndpointResource)
  @delete
  @autoRoute
  @requiresPermissions(#[Permission.webhooksManage])
  delete(...ResourceParameters<SiteWebhookEndpointResource>):
    | NoContentResponse
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem;

  /** Replace the signing secret. The old one stops verifying at once; the new one is in the response. Owners and admins only. */
  @action("regenerate-secret")
  @autoRoute
  @requiresPermissions(#[Permission.webhooksManage])
  regenerateSecret(...ResourceParameters<SiteWebhookEndpointResource>):
    | SiteWebhookEndpointResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem;

  /** Send a sample event of the chosen type, signed as a real delivery, and return the receiver's answer. Owners and admins only. */
  @action("test")
  @autoRoute
  @requiresPermissions(#[Permission.webhooksManage])
  test(
    ...ResourceParameters<SiteWebhookEndpointResource>,
    @body body: TestWebhookInput,
  ):
    | SiteWebhookTestResult
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;
}
