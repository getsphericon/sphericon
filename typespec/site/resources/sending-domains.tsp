using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

// The resource is common/sending-domains.tsp's; /site addresses it under the Workspace.
@@parentResource(SendingDomainResource, SiteWorkspaceResource);

@tag("Site Sending Domains")
interface SiteSendingDomains {
  /** List sending domains for the site UI */
  @listsResource(SendingDomainResource)
  @get
  @autoRoute
  @requiresPermissions(#[])
  list(...ResourceCollectionParameters<SendingDomainResource>, ...PageQuery):
    | Page<SendingDomainResource>
    | Problem;

  /** Add a sending domain; mints the DKIM keypair and returns the DNS records. Owners and admins only. */
  @createsResource(SendingDomainResource)
  @post
  @autoRoute
  @requiresPermissions(#[Permission.sendingDomainsManage])
  create(
    ...ResourceCollectionParameters<SendingDomainResource>,
    @body body: CreateSendingDomainInput,
  ):
    | {
        @statusCode statusCode: 201;
        @body body: SendingDomainResource;
      }
    | Problem;

  /** Get a sending domain by ID for the site UI, deleted or not */
  @readsResource(SendingDomainResource)
  @get
  @autoRoute
  @requiresPermissions(#[])
  get(...ResourceParameters<SendingDomainResource>):
    | SendingDomainResource
    | Problem;

  /**
   * Delete a sending domain for good: it leaves the list and is never restored; what it sent keeps its history.
   * Refused with 409 while a scheduled or sending broadcast or an active automation sends from it. Owners and admins only.
   */
  @deletesResource(SendingDomainResource)
  @delete
  @autoRoute
  @requiresPermissions(#[Permission.sendingDomainsManage])
  delete(...ResourceParameters<SendingDomainResource>):
    | NoContentResponse
    | Problem;

  /**
   * Re-check the DNS now and answer once the check is done: `verified` and `lastCheckedAt` of
   * the returned domain are the outcome; a deleted domain is never checked again (404). Owners and
   * admins only.
   */
  @action("verify")
  @autoRoute
  @requiresPermissions(#[Permission.sendingDomainsManage])
  verify(...ResourceParameters<SendingDomainResource>):
    | SendingDomainResource
    | Problem;
}
