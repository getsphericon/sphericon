using TypeSpec.Http;
using TypeSpec.OpenAPI;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

namespace Sphericon;

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

  /** Whether the endpoint receives deliveries: true while it has no `disabledAt` */
  enabled: boolean;

  /** When the endpoint was turned off; absent while it is enabled */
  disabledAt?: Timestamp;

  /** Why the endpoint is off; present exactly when `disabledAt` is */
  disabledReason?: WebhookDisabledReason;

  /** Present while deliveries are failing; cleared by the next 2xx */
  failure?: WebhookFailure;

  /** The revision of the state this answer shows (ADR 0119): an update names it */
  revision: Revision;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

@tag("Webhooks")
interface Webhooks
  extends ExternalCrud<
      WebhookResource,
      CreateWebhookInput,
      UpdateWebhookInput,
      ApiTokenScope.webhooksRead,
      ApiTokenScope.webhooksWrite,
      ApiTokenScope.webhooksWrite
    > {
  /** Replace the endpoint's signing secret; the old one stops verifying at once. The new secret is never returned here: read it in the app. Needs the webhooks:regenerate scope. */
  @action("regenerate-secret")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.webhooksRegenerate])
  regenerateSecret(...ResourceParameters<WebhookResource>):
    | WebhookResource
    | Problem;

  /** Send a sample event of the chosen type, signed as a real delivery, and return the receiver's answer. Does not change the endpoint's failure state. Needs the webhooks:test scope. */
  @action("test")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.webhooksTest])
  test(...ResourceParameters<WebhookResource>, @body body: TestWebhookInput):
    | WebhookTestResult
    | Problem;
}
