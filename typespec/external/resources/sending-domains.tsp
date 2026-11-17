using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/**
 * Change a Sending domain. The domain name is immutable; changing the selector moves the DKIM
 * record, so the domain becomes unverified until the new record is published. The app has no
 * such edit yet, so this body is /api's own.
 */
model UpdateSendingDomainInput {
  /** New DKIM selector */
  dkimSelector?: string;
}

@tag("Sending domains")
interface SendingDomains {
  /** List resources with pagination */
  @listsResource(SendingDomainResource)
  @get
  @autoRoute
  @requiresScopes(#[ApiTokenScope.sendingDomainsRead])
  list(...ResourceCollectionParameters<SendingDomainResource>, ...PageQuery):
    | Page<SendingDomainResource>
    | Problem;

  /** Create a resource */
  @createsResource(SendingDomainResource)
  @post
  @autoRoute
  @requiresScopes(#[ApiTokenScope.sendingDomainsWrite])
  create(
    ...ResourceCollectionParameters<SendingDomainResource>,
    @body body: CreateSendingDomainInput,
  ):
    | {
        @statusCode statusCode: 201;
        @body body: SendingDomainResource;
      }
    | Problem;

  /** Get a resource by ID */
  @readsResource(SendingDomainResource)
  @get
  @autoRoute
  @requiresScopes(#[ApiTokenScope.sendingDomainsRead])
  get(...ResourceParameters<SendingDomainResource>):
    | SendingDomainResource
    | Problem;

  /** Update a Sending domain; a deleted one is read-only and answers 404 */
  @updatesResource(SendingDomainResource)
  @put
  @autoRoute
  @requiresScopes(#[ApiTokenScope.sendingDomainsWrite])
  update(
    ...ResourceParameters<SendingDomainResource>,
    @body body: UpdateSendingDomainInput,
  ): SendingDomainResource | Problem;

  /** Delete a Sending domain for good: it leaves the list and is never restored; what it sent keeps its history. Refused with 409 (`usedBy`) while a scheduled or sending Broadcast or an active Automation sends from it. */
  @deletesResource(SendingDomainResource)
  @delete
  @autoRoute
  @requiresScopes(#[ApiTokenScope.sendingDomainsWrite])
  delete(...ResourceParameters<SendingDomainResource>):
    | NoContentResponse
    | Problem;

  /** Run a live DNS check now and answer once it is done: the domain as the check left it (`verified`, `lastCheckedAt`). Verification is never set by the caller, and a deleted domain is never checked again (404). */
  @action("verify")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.sendingDomainsWrite])
  verify(...ResourceParameters<SendingDomainResource>):
    | SendingDomainResource
    | Problem;
}
