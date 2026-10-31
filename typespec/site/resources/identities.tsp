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

/** The Linked identities of the authenticated User */
model SiteLinkedIdentities {
  items: SiteLinkedIdentity[];
}

@route("/me/identities")
@tag("Site Linked Identities")
namespace SiteIdentities {
  /** List the authenticated User's Linked identities */
  @get
  op list(): {
    @statusCode statusCode: 200;
    @body body: SiteLinkedIdentities;
  };

  /** Start linking another Sign-in provider account to the authenticated User. Works like
   * `start` of a sign-in, but the flow cookie also names the User, and the shared callback
   * adds the provider's subject to that User whatever email the provider reports. The
   * callback redirects to `return_to` (default the account security page); a refusal goes
   * to the account security page with the `error` code (see SiteSigninError). 404 for a
   * provider that is not configured. */
  @get
  @route("/{provider}/link")
  op link(
    @path provider: string,

    /** A relative in-app path to land on afterwards */
    @query(#{ name: "return_to" }) returnTo?: string,
  ):
    | {
        @statusCode statusCode: 302;

        /** The provider's authorization URL */
        @header("Location") location: string;

        /** The flow cookie */
        @header("Set-Cookie") setCookie: string;
      }
    | BadRequestProblem
    | NotFoundProblem;

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
    | BadRequestProblem
    | NotFoundProblem
    | ConflictProblem;
}
