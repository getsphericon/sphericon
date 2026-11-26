using TypeSpec.Http;

namespace Sphericon;

/** A Link identity token the tracker found on the page's address (ADR 0160), presented with the
 * visitor on the person's first interaction with the page */
model CollectLinkIdentityInput {
  /** The tracker's id of the visitor, created when it has not been seen */
  @minLength(1)
  @maxLength(255)
  visitorId: string;

  /** The token of the `sph_lit` query parameter a tracked link's click added to the address */
  @minLength(1)
  @maxLength(4096)
  token: string;
}

@route("/link-identities")
@tag("Collect")
namespace CollectLinkIdentities {
  /** Binds the visitor to the contact a Link identity token names, as a claim binds: only while the
   * visitor is anonymous or already that contact's, stitching its anonymous events onto the contact
   * and handing it none of the visitor's chats. It answers 204 whatever it binds: an invalid,
   * expired or foreign token, a contact that no longer exists, a visitor of another contact, or a
   * request whose User-Agent names a known crawler binds nothing and says so to no one. */
  @post
  op create(@body body: CollectLinkIdentityInput): NoContentResponse | Problem;
}
