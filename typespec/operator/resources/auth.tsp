using TypeSpec.Http;

namespace Sphericon;

/** The authenticated platform Operator (shown to customers only as "sphericon staff") */
model OperatorResource {
  /** Unique identifier */
  id: EntityId;

  /** Login email */
  email: EmailAddress;
}

/** Sign in with an email and password. The email is a plain string: a malformed
 * address must answer like an unknown one. */
model OperatorLoginInput {
  email: string;
  password: string;
}

/** What the password step granted. There is never a session at this step: a TOTP is
 * mandatory (ADR 0026). `challenge`: the Operator has a TOTP, go on with a code.
 * `enrolment`: the Operator has none yet (first login); the `enrolment` secret is to be
 * added to an authenticator app, and the first code confirms it. */
enum OperatorLoginOutcome {
  challenge,
  enrolment,
}

/** A TOTP secret awaiting its first confirming code */
model OperatorEnrolment {
  /** Base32 key */
  secret: string;

  /** otpauth:// URI */
  uri: string;

  /** PNG data URI of the URI */
  qrCode: string;
}

model OperatorLoginResult {
  outcome: OperatorLoginOutcome;

  /** The signed challenge for the second step: bound to the Operator, valid for 5
   * minutes, working for one successful second step. */
  challenge: string;

  /** Present when the outcome is `enrolment` */
  enrolment?: OperatorEnrolment;
}

/** The second login step: the challenge from the password step and a TOTP code */
model OperatorSecondFactorInput {
  challenge: string;

  /** A current TOTP code from the authenticator app */
  code: string;
}

@route("/auth")
@tag("Operator Auth")
namespace OperatorAuth {
  /** Check the password. Answers a challenge (or, at first login, a TOTP enrolment)
   * and never a session. Unknown email and wrong password answer the same 401. */
  @useAuth(NoAuth)
  @post
  @route("/login")
  op login(@body body: OperatorLoginInput):
    | {
        @statusCode statusCode: 200;
        @body body: OperatorLoginResult;
      }
    | UnauthorizedProblem
    | TooManyRequestsProblem;

  /** The second login step: verify the challenge and a TOTP code, then start the
   * session (the cookie is set here). At first login the code also confirms the
   * enrolment. An expired, reused or forged challenge and a wrong code answer the
   * same 401. */
  @useAuth(NoAuth)
  @post
  @route("/second-factor")
  op secondFactor(@body body: OperatorSecondFactorInput):
    | {
        @statusCode statusCode: 200;

        /** The Operator session cookie */
        @header("Set-Cookie") setCookie: string;

        @body body: OperatorResource;
      }
    | UnauthorizedProblem
    | TooManyRequestsProblem;

  /** End the session: bumps the Operator's session epoch, so every session token of
   * the Operator (a copied one too) stops verifying, and clears the session cookie. */
  @post
  @route("/logout")
  op logout():
    | {
        @statusCode statusCode: 204;

        /** The expired session cookie */
        @header("Set-Cookie") setCookie: string;
      }
    | UnauthorizedProblem;
}

@tag("Operator")
namespace OperatorMe {
  /** The authenticated Operator */
  @get
  @route("/me")
  op get(): OperatorResource | UnauthorizedProblem;
}
