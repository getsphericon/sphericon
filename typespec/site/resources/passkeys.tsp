using TypeSpec.Http;

namespace Sphericon;

/** The relying party of a Passkey ceremony: this Instance */
model SitePasskeyRelyingParty {
  /** The relying party id: the host of the Instance's public URL */
  id?: string;

  name: string;
}

/** The account a Passkey is created for */
model SitePasskeyUserEntity {
  /** The user handle, base64url */
  id: string;

  name: string;
  displayName: string;
}

/** A public-key algorithm the authenticator may use */
model SitePasskeyParameter {
  type: "public-key";

  /** The COSE algorithm identifier */
  alg: int32;
}

/** A credential, as the exclusion list names it */
model SitePasskeyDescriptor {
  type: "public-key";

  /** The credential id, base64url */
  id: string;

  transports?: ("ble" | "hybrid" | "internal" | "nfc" | "usb")[];
}

/** What the authenticator must be able to do. Credentials are discoverable and user
 * verification is required (ADR 0051). */
model SitePasskeyAuthenticatorSelection {
  authenticatorAttachment?: "platform" | "cross-platform";
  requireResidentKey?: boolean;
  residentKey?: "discouraged" | "preferred" | "required";
  userVerification?: "required" | "preferred" | "discouraged";
}

/** The options a browser passes to `navigator.credentials.create` to register a Passkey: the
 * W3C `PublicKeyCredentialCreationOptionsJSON` dictionary, as the WebAuthn library produced it.
 * Attestation is not requested. */
model SitePasskeyCreationOptions {
  rp: SitePasskeyRelyingParty;
  user: SitePasskeyUserEntity;

  /** The challenge, base64url */
  challenge: string;

  pubKeyCredParams: SitePasskeyParameter[];

  /** Milliseconds the ceremony may take */
  timeout?: int32;

  /** The Passkeys the User already holds, so one authenticator is not registered twice */
  excludeCredentials?: SitePasskeyDescriptor[];

  authenticatorSelection?: SitePasskeyAuthenticatorSelection;
  attestation?: "none" | "indirect" | "direct" | "enterprise";
}

/** The browser's answer to the creation options: the W3C `RegistrationResponseJSON`
 * dictionary (`PublicKeyCredential.toJSON()`), checked by the WebAuthn library. */
model SitePasskeyRegistrationResponse is Record<unknown>;

/** One Passkey of the authenticated User (ADR 0051). Its public key and credential id are
 * never returned. */
model SitePasskey {
  /** Unique identifier, the key of the rename and remove operations */
  id: EntityId;

  /** What the User calls it, to tell their Passkeys apart */
  name: string;

  /** When the Passkey was registered */
  createdAt: Timestamp;

  /** When a login last used it; absent while it has never been used */
  lastUsedAt?: Timestamp;
}

/** The Passkeys of the authenticated User, oldest first, and what proves the User for a change */
model SitePasskeyList {
  items: SitePasskey[];

  /** Whether Passkeys work on this Instance. They need a host name: an Instance served on an
   * IP address has none, and the registration operations answer 404. */
  available: boolean;

  /** Whether the User has a password. Without one, adding a Passkey asks for a fresh
   * sign-in with a Sign-in provider instead. */
  passwordSet: boolean;

  /** Whether the session holds a fresh proof that stands in for the password: a Sign-in
   * provider login or a Passkey check of the last few minutes (a User with a password always
   * proves the password). */
  identityProofFresh: boolean;
}

/** The new name of a Passkey */
model SitePasskeyRenameInput {
  /** What to call the Passkey, to tell it from the User's others */
  name: string;
}

/** The proof of identity a removal asks for */
model SitePasskeyRemoveInput {
  /** Current password. Left out by a User with no password, who proves a provider or Passkey
   * sign-in of the last few minutes instead (ADR 0048). */
  currentPassword?: string;
}

/** The options to check a Passkey with, and the ceremony cookie they are bound to */
model SitePasskeyCheckBegun {
  options: SitePasskeyRequestOptions;
}

/** The browser's response to a Passkey check ceremony */
model SitePasskeyCheckInput {
  /** The authenticator's response to the request options */
  credential: SitePasskeyAssertion;
}

/** The options to create a Passkey with, and the ceremony cookie they are bound to */
model SitePasskeyRegistrationBegun {
  options: SitePasskeyCreationOptions;
}

/** The browser's response to a registration ceremony, with the proof of identity and a name */
model SitePasskeyRegistrationInput {
  /** Current password. Left out by a User with no password, who proves a provider or Passkey
   * sign-in of the last few minutes instead (ADR 0048). */
  currentPassword?: string;

  /** What to call the Passkey, to tell it from the User's others */
  name: string;

  /** The authenticator's response to the creation options */
  credential: SitePasskeyRegistrationResponse;
}

@route("/me/passkeys")
@tag("Site Passkeys")
namespace SitePasskeys {
  /** List the authenticated User's Passkeys */
  @get
  op list(): {
    @statusCode statusCode: 200;
    @body body: SitePasskeyList;
  };

  /** Begin registering a Passkey: the creation options for the browser, with the challenge
   * in a signed, httpOnly cookie. Nothing is written: the ceremony is stateless until it
   * finishes (ADR 0051). The options exclude the Passkeys the User already holds. */
  @post
  @route("/registration")
  op beginRegistration():
    | {
        @statusCode statusCode: 200;

        /** The ceremony cookie carrying the challenge */
        @header("Set-Cookie") setCookie: string;

        @body body: SitePasskeyRegistrationBegun;
      }
    | NotFoundProblem;

  /** Finish registering a Passkey, proving the password (or a fresh provider login or Passkey check). The
   * authenticator must have verified the user and the challenge must not have been used
   * before. On success every other session ends, the acting one continues under the cookie
   * set here and the owner is mailed. 403 on a wrong password (it feeds the Login throttle:
   * 429 while its delay runs), 422 when the ceremony fails: no ceremony cookie, an expired
   * or spent challenge, a response the library refuses, an authenticator that did not verify
   * the user, or a credential already registered. */
  @post
  @route("/registration/finish")
  op finishRegistration(
    @body body: SitePasskeyRegistrationInput,

    /** The ceremony cookie set by `beginRegistration` */
    @cookie("PASSKEY") ceremony?: string,
  ):
    | {
        @statusCode statusCode: 200;

        /** The acting session reissued under the new session epoch */
        @header("Set-Cookie") setCookie: string;

        @body body: SitePasskey;
      }
    | ForbiddenProblem
    | NotFoundProblem
    | UnprocessableEntityProblem
    | TooManyRequestsProblem;

  /** Begin a fresh Passkey check: the request options for the browser, naming the User's own
   * Passkeys, with the challenge in a signed, httpOnly cookie. Nothing is written. The check
   * stands in for the password of a User who has none (ADR 0052). 404 when Passkeys are
   * unavailable or the User holds none. */
  @post
  @route("/check")
  op beginCheck():
    | {
        @statusCode statusCode: 200;

        /** The ceremony cookie carrying the challenge */
        @header("Set-Cookie") setCookie: string;

        @body body: SitePasskeyCheckBegun;
      }
    | NotFoundProblem;

  /** Finish a Passkey check. The authenticator must have verified the person, by a Passkey of
   * the session's own User, against a challenge not used before. The acting session is
   * reissued carrying the time of the check, which counts as the proof of identity for the
   * next few minutes; no session ends. 403 for any response that does not verify. */
  @post
  @route("/check/finish")
  op finishCheck(
    @body body: SitePasskeyCheckInput,

    /** The ceremony cookie set by `beginCheck` */
    @cookie("PASSKEY") ceremony?: string,
  ):
    | {
        @statusCode statusCode: 204;

        /** The acting session reissued with the check */
        @header("Set-Cookie") setCookie: string;
      }
    | ForbiddenProblem
    | NotFoundProblem;

  /** Rename a Passkey of the authenticated User. 404 for an unknown Passkey or another User's,
   * 422 for a name that is empty or longer than 100 characters. */
  @patch
  @route("/{id}")
  op rename(@path id: EntityId, @body body: SitePasskeyRenameInput):
    | {
        @statusCode statusCode: 200;
        @body body: SitePasskey;
      }
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Remove a Passkey of the authenticated User, proving the password (or a fresh provider
   * login). It no longer signs anyone in, every other session ends, the acting one continues
   * under the cookie set here and the owner is mailed. 403 on a wrong password (it feeds the
   * Login throttle: 429 while its delay runs), 404 for an unknown Passkey or another User's,
   * 409 when it is the last way the User can sign in: no password, no Linked identity and no
   * other Passkey. */
  @delete
  @route("/{id}")
  op remove(@path id: EntityId, @body body: SitePasskeyRemoveInput):
    | {
        @statusCode statusCode: 204;

        /** The acting session reissued under the new session epoch */
        @header("Set-Cookie") setCookie: string;
      }
    | ForbiddenProblem
    | NotFoundProblem
    | ConflictProblem
    | TooManyRequestsProblem;
}
