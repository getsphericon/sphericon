using TypeSpec.Http;

namespace Sphericon;

/** The authenticated platform Staff member (shown to customers only as "Sphericon staff") */
model StaffResource {
  /** Unique identifier */
  id: EntityId;

  /** Login email */
  email: EmailAddress;
}

/** Sign in with an email and password. The email is a plain string: a malformed
 * address must answer like an unknown one. */
model StaffLoginInput {
  email: string;
  password: string;
}

/** What the password step granted. There is never a session at this step: a TOTP is
 * mandatory (ADR 0026). `challenge`: the Staff member has a TOTP, go on with a code.
 * `enrolment`: the Staff member has none yet (first login); the `enrolment` secret is to be
 * added to an authenticator app, and the first code confirms it. */
enum StaffLoginOutcome {
  challenge,
  enrolment,
}

/** A TOTP secret awaiting its first confirming code */
model StaffEnrolment {
  /** Base32 key */
  secret: string;

  /** otpauth:// URI */
  uri: string;

  /** PNG data URI of the URI */
  qrCode: string;
}

model StaffLoginResult {
  outcome: StaffLoginOutcome;

  /** The signed challenge for the second step: bound to the Staff member, valid for 5
   * minutes, working for one successful second step. */
  challenge: string;

  /** Present when the outcome is `enrolment` */
  enrolment?: StaffEnrolment;
}

/** The second login step: the challenge from the password step and a TOTP code */
model StaffSecondFactorInput {
  challenge: string;

  /** A current TOTP code from the authenticator app */
  code: string;
}

@route("/auth")
@tag("StaffMember Auth")
namespace StaffAuth {
  /** Check the password. Answers a challenge (or, at first login, a TOTP enrolment)
   * and never a session. Unknown email and wrong password answer the same 401. */
  @useAuth(NoAuth)
  @post
  @route("/login")
  op login(@body body: StaffLoginInput):
    | {
        @statusCode statusCode: 200;
        @body body: StaffLoginResult;
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
  op secondFactor(@body body: StaffSecondFactorInput):
    | {
        @statusCode statusCode: 200;

        /** The Staff member session cookie */
        @header("Set-Cookie") setCookie: string;

        @body body: StaffResource;
      }
    | UnauthorizedProblem
    | TooManyRequestsProblem;

  /** End the session: bumps the Staff member's session epoch, so every session token of
   * the Staff member (a copied one too) stops verifying, and clears the session cookie. */
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

@tag("StaffMember")
namespace StaffMe {
  /** The authenticated Staff member */
  @get
  @route("/me")
  op get(): StaffResource | UnauthorizedProblem;
}
