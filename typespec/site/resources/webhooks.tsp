using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

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

/**
 * Webhook endpoints are managed by owners and admins only: the resource carries the
 * signing secret, so every operation, reads included, is refused to a member with 403.
 */
@tag("Site Webhooks")
interface SiteWebhooks
  extends WorkspaceCrud<
      SiteWebhookEndpointResource,
      CreateWebhookInput,
      UpdateWebhookInput,
      #[Permission.webhooksManage],
      #[Permission.webhooksManage],
      #[Permission.webhooksManage]
    > {
  /** Replace the signing secret. The old one stops verifying at once; the new one is in the response. Owners and admins only. */
  @action("regenerate-secret")
  @autoRoute
  @requiresPermissions(#[Permission.webhooksManage])
  regenerateSecret(...ResourceParameters<SiteWebhookEndpointResource>):
    | SiteWebhookEndpointResource
    | Problem;

  /** Send a sample event of the chosen type, signed as a real delivery, and return the receiver's answer. Owners and admins only. */
  @action("test")
  @autoRoute
  @requiresPermissions(#[Permission.webhooksManage])
  test(
    ...ResourceParameters<SiteWebhookEndpointResource>,
    @body body: TestWebhookInput,
  ): WebhookTestResult | Problem;
}

@@doc(SiteWebhooks.list, "List webhook endpoints. Owners and admins only.");
@@doc(
  SiteWebhooks.create,
  "Create a webhook endpoint. Owners and admins only."
);
@@doc(
  SiteWebhooks.get,
  "Get a webhook endpoint by ID. Owners and admins only."
);
@@doc(
  SiteWebhooks.update,
  "Update a webhook endpoint. Owners and admins only."
);
@@doc(
  SiteWebhooks.delete,
  "Delete a webhook endpoint. Owners and admins only."
);
