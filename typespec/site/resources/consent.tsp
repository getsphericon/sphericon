using TypeSpec.Http;

namespace Sphericon;

// Public, session-less actions behind the signed tokens in marketing emails. The
// token is the credential, so these are NoAuth and not workspace-scoped. The email
// links themselves (GET /e/u/{token}, GET /e/confirm/{token}) stay outside this
// contract: they only redirect to the SPA pages that call these operations.

@route("/confirmations")
@tag("Site Consent")
namespace SitePublicConfirmations {
  /** Perform a double opt-in confirmation (ADR 0013). The deliberate human act
   * behind the confirmation page's button. Repeating it is a no-op. 410 means the
   * link has expired, so the page offers to sign up again; 400 is an invalid token. */
  @useAuth(NoAuth)
  @post
  @route("/{token}")
  op perform(@path token: string): NoContentResponse | Problem;
}

@route("/unsubscribes")
@tag("Site Consent")
namespace SitePublicUnsubscribes {
  /** Perform an unsubscribe (ADR 0012). The button on the unsubscribe page; the
   * mailbox provider one-click POST keeps using POST /e/u/{token}. Repeating it
   * is a no-op. */
  @useAuth(NoAuth)
  @post
  @route("/{token}")
  op perform(@path token: string): NoContentResponse | Problem;
}
