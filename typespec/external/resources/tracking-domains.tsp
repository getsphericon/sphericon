using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

@route("/tracking-domains")
@tag("Tracking domains")
namespace TrackingDomains {
  /** List the Workspace's Tracking domains: the current one, the replacement, then the retired ones. Empty while the Instance offers none. */
  @get
  @requiresScopes(#[ApiTokenScope.trackingDomainsRead])
  op list(): TrackingDomainResource[] | Problem;

  /** Set the Workspace's Tracking domain. With none it becomes the current one. While the current one serves it becomes the replacement (discarding a pending one) and takes over once it serves. When the current one does not serve, the new one replaces it at once. A host the Workspace already uses is returned as it is; a retired one is brought back. An invalid host or one of the Instance's own is 422; 404 when the Instance offers no Tracking domains. */
  @post
  @requiresScopes(#[ApiTokenScope.trackingDomainsWrite])
  op set(@body body: SetTrackingDomainInput): TrackingDomainResource | Problem;

  /** Check the host now and answer once the check is done: the Tracking domain as it left it. A replacement that reaches `serving` becomes current. Verification is never set by the caller. */
  @post
  @route("/{id}/verify")
  @requiresScopes(#[ApiTokenScope.trackingDomainsWrite])
  op verify(@path id: EntityId): TrackingDomainResource | Problem;

  /** Remove a Tracking domain by its role. The current one stops being used: it is retired when it served (links already sent on it keep working while its CNAME points at the edge), deleted otherwise, and a replacement becomes current. A replacement is cancelled. A retired one is forgotten: links sent on it stop working. */
  @delete
  @route("/{id}")
  @requiresScopes(#[ApiTokenScope.trackingDomainsWrite])
  op remove(@path id: EntityId): NoContentResponse | Problem;
}
