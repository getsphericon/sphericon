using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** The workspace's identity secret (ADR 0171): its server signs identity tokens with it (HS256), so
 * an identify from its site proves who is signed in. Owners and admins only */
model SiteIdentitySecretResource {
  /** The secret; absent until an owner or admin generates one */
  secret?: string;
}

@route("/workspaces/{workspaceId}/identity-secret")
@tag("Site Identity Secret")
namespace SiteIdentitySecret {
  /** The workspace's identity secret. Owners and admins only */
  @get
  @requiresPermissions(#[Permission.tokensManage])
  op get(@path workspaceId: WorkspaceId): SiteIdentitySecretResource | Problem;

  /** Generate the identity secret, or replace it: the one before stops verifying at once, and the
   * new one is in the response. Owners and admins only */
  @post
  @route("/regenerate")
  @requiresPermissions(#[Permission.tokensManage])
  op regenerate(@path workspaceId: WorkspaceId):
    | SiteIdentitySecretResource
    | Problem;
}
