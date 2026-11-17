using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

@route("/workspaces/{workspaceId}/tracking-domains")
@tag("Site Tracking Domains")
namespace SiteTrackingDomains {
  /** List the Workspace's Tracking domains: the current one, the replacement, then the retired ones. Empty while the Instance offers none. */
  @get
  @requiresPermissions(#[])
  op list(@path workspaceId: WorkspaceId): TrackingDomainResource[] | Problem;

  /** Set the Workspace's Tracking domain (owner and admin only). With none it becomes the current one. While the current one serves it becomes the replacement (discarding a pending one) and takes over once it serves. When the current one does not serve, the new one replaces it at once. A host the Workspace already uses is returned as it is; a retired one is brought back. An invalid host or one of the Instance's own is 422; 404 when the Instance offers no Tracking domains. */
  @post
  @requiresPermissions(#[Permission.trackingDomainsManage])
  op set(@path workspaceId: WorkspaceId, @body body: SetTrackingDomainInput):
    | TrackingDomainResource
    | Problem;

  /** Check the host now (owner and admin only) and answer once the check is done: the Tracking domain as it left it. A replacement that reaches `serving` becomes current. */
  @post
  @route("/{id}/verify")
  @requiresPermissions(#[Permission.trackingDomainsManage])
  op verify(@path workspaceId: WorkspaceId, @path id: EntityId):
    | TrackingDomainResource
    | Problem;

  /** Remove a Tracking domain by its role (owner and admin only). The current one stops being used: it is retired when it served (links already sent on it keep working while its CNAME points at the edge), deleted otherwise, and a replacement becomes current. A replacement is cancelled. A retired one is forgotten: links sent on it stop working. */
  @delete
  @route("/{id}")
  @requiresPermissions(#[Permission.trackingDomainsManage])
  op remove(@path workspaceId: WorkspaceId, @path id: EntityId):
    | NoContentResponse
    | Problem;
}
