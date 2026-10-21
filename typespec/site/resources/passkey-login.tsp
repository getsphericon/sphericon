using TypeSpec.Http;

namespace Sphericon;

/** The options a browser passes to `navigator.credentials.get` to log in with a Passkey: the
 * W3C `PublicKeyCredentialRequestOptionsJSON` dictionary, as the WebAuthn library produced it.
 * `allowCredentials` is empty: the User is found by the credential the browser offers, never by
 * an email (ADR 0052). */
model SitePasskeyRequestOptions {
  /** The challenge, base64url */
  challenge: string;

  /** Milliseconds the ceremony may take */
  timeout?: int32;

  /** The relying party id: the host of the Instance's public URL */
  rpId?: string;

  allowCredentials?: SitePasskeyDescriptor[];

  /** Always `required`: an authenticator that cannot verify the person is refused */
  userVerification?: "required" | "preferred" | "discouraged";
}

/** The options to log in with, and the ceremony cookie they are bound to */
model SitePasskeyLoginBegun {
  options: SitePasskeyRequestOptions;
}

/** The browser's answer to the request options: the W3C `AuthenticationResponseJSON`
 * dictionary (`PublicKeyCredential.toJSON()`), checked by the WebAuthn library. */
model SitePasskeyAssertion is Record<unknown>;

/** The browser's response to a login ceremony */
model SitePasskeyLoginInput {
  /** The authenticator's response to the request options */
  credential: SitePasskeyAssertion;
}

@route("/auth/passkey")
@tag("Site Auth")
namespace SitePasskeyLogin {
  /** Begin a Passkey login: the request options for the browser, with the challenge in a signed,
   * httpOnly cookie. Anonymous and stateless: nothing is written until the login finishes, so
   * the call cannot be used to fill a table (ADR 0052). 404 when the Instance has no host name
   * a Passkey can be bound to. */
  @useAuth(NoAuth)
  @post
  op begin():
    | {
        @statusCode statusCode: 200;

        /** The ceremony cookie carrying the challenge */
        @header("Set-Cookie") setCookie: string;

        @body body: SitePasskeyLoginBegun;
      }
    | NotFoundProblem;

  /** Finish a Passkey login: the authenticator's response, with no email and no password. The
   * User is found by the credential id; the authenticator must have verified the person and the
   * challenge must not have been used before. A Passkey replaces the password and the Second
   * factor, so the session cookie is set at once (ADR 0052). Every failure (an unknown credential,
   * a response that does not verify, a spent or expired challenge, an Unverified User) answers
   * the same 401 and counts against no one; while a known User's Login throttle delay runs the
   * answer is 429 (ADR 0025). */
  @useAuth(NoAuth)
  @post
  @route("/finish")
  op finish(
    @body body: SitePasskeyLoginInput,

    /** The ceremony cookie set by `begin` */
    @cookie("PASSKEY") ceremony?: string,
  ):
    | {
        @statusCode statusCode: 200;

        /** The session cookie (always set here; optional only because the response shares its
         * shape with the login operation's) */
        @header("Set-Cookie") setCookie?: string;

        @body body: SiteLoginResult;
      }
    | UnauthorizedProblem
    | TooManyRequestsProblem;
}
