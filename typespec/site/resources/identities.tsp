using TypeSpec.Http;

namespace Sphericon;

/** One Linked identity of the authenticated User (ADR 0048): a Sign-in provider the User
 * can sign in with. The provider's subject is never returned. */
model SiteLinkedIdentity {
  /** Unique identifier, the key of the unlink operation */
  id: EntityId;

  /** The provider's registry name */
  provider: string;

  /** What the provider is called to a person (the registry name when the Instance no longer offers it) */
  displayName: string;

  /** When the identity was linked */
  createdAt: Timestamp;
}

/** The Linked identities of the authenticated User, and what proves the User for a link */
model SiteLinkedIdentities {
  items: SiteLinkedIdentity[];

  /** Whether the User has a password. Without one, linking a provider asks for a fresh
   * sign-in with a Sign-in provider or a Passkey check instead. */
  passwordSet: boolean;

  /** Whether the session holds a fresh proof that stands in for the password: a Sign-in
   * provider login or a Passkey check of the last few minutes (a User with a password always
   * proves the password). */
  identityProofFresh: boolean;
}

/** The provider to link, and the proof of identity a link asks for */
model SiteIdentityLinkInput {
  /** The registry name of the Sign-in provider to link */
  provider: string;

  ...SiteReauthentication;
}

/** Where to send the browser to link the provider account: the provider's authorization URL */
model SiteIdentityLinkStarted {
  redirectUrl: string;
}

@route("/me/identities")
@tag("Site Linked Identities")
namespace SiteIdentities {
  /** List the authenticated User's Linked identities */
  @get
  op list():
    | {
        @statusCode statusCode: 200;
        @body body: SiteLinkedIdentities;
      }
    | Problem;

  /** Start linking another Sign-in provider account to the authenticated User, proving the
   * password (or a fresh provider login or Passkey check) as adding a Passkey does. Works like
   * `start` of a sign-in, but the flow cookie also names the User, and the shared callback
   * adds the provider's subject to that User whatever email the provider reports; the owner
   * is then mailed a notice with a link that undoes it. The browser is sent on to
   * `redirectUrl`; the callback lands on the account security page, with the `error` code
   * of a refusal (see SiteSigninError). 403 on a wrong or missing proof (a wrong password
   * feeds the Login throttle: 429 while its delay runs), 404 for a provider that is not
   * configured. */
  @post
  @route("/link")
  op link(@body body: SiteIdentityLinkInput):
    | {
        @statusCode statusCode: 200;

        /** The flow cookie */
        @header("Set-Cookie") setCookie: string;

        @body body: SiteIdentityLinkStarted;
      }
    | Problem;

  /** Unlink an identity. 409 when it would leave the User no way to sign in (no password
   * and no other identity). Every other session ends; the acting one continues under the
   * cookie set here. */
  @delete
  @route("/{id}")
  op unlink(@path id: EntityId):
    | {
        @statusCode statusCode: 204;

        /** The acting session reissued under the new session epoch */
        @header("Set-Cookie") setCookie: string;
      }
    | Problem;
}
