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
  @rateLimited("human")
  op perform(@path token: string): NoContentResponse | Problem;

  /** Read what the confirmation page shows for a link (ADR 0013): whether it has expired, the
   * Workspace's name and its Brand kit as it stands. A link past its 7 days still names its
   * Workspace, so the page of an expired link is in its brand and offers a new one; 400 is an
   * invalid token. */
  @useAuth(NoAuth)
  @get
  @route("/{token}")
  @rateLimited("human")
  op read(@path token: string): SiteConfirmationPage | Problem;

  /** Send a new confirmation link, from the page of an expired one (ADR 0013): a request the person
   * makes, which only a Suppression of the address stops. The answer is the same whether or not a
   * mail goes out (an address over its limit, suppressed, or no longer the contact's), so the page
   * says nothing about an address; `confirmed` when it holds a Confirmation already. 409
   * `confirmation_email_required` while the Workspace has no confirmation email set up. */
  @useAuth(NoAuth)
  @post
  @route("/{token}/renewals")
  @rateLimited("human")
  op renew(@path token: string):
    | {
        @statusCode statusCode: 202;
        @body body: ConfirmationRequestResult;
      }
    | Problem;
}

/** The Brand kit of a public page, as it stands */
model SitePublicBrand {
  /** The brand color: buttons and links, #rrggbb */
  accentColor: string;

  /** The color of text, #rrggbb */
  textColor: string;

  /** The page's background color, #rrggbb */
  backgroundColor: string;

  /** The CSS font-family list of body text */
  bodyFont: string;

  /** The text color of a button, #rrggbb */
  buttonTextColor: string;

  /** The corner radius of a button, in pixels */
  buttonRadius: int32;

  /** The absolute URL of the logo; empty for none */
  logoUrl: string;
}

/** What the confirmation page of a link shows */
model SiteConfirmationPage {
  /** The link is past its 7 days: it confirms nothing, and the page offers a new one */
  expired: boolean;

  /** The name of the Workspace that sent the link */
  workspaceName: string;

  /** The Workspace's Brand kit, read now */
  brand: SitePublicBrand;
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
