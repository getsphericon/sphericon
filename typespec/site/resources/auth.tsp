using TypeSpec.Http;

namespace Sphericon;

/** Register with a name and an email: no password is taken before the address is
 * proven (ADR 0030). Only an install with `SPHERICON_REQUIRE_EMAIL_VERIFICATION=false` takes a
 * password here. */
model SiteRegisterInput {
  name: string;
  email: EmailAddress;

  /** Required when the install does not require email verification, where it makes
   * the account usable at once. Otherwise ignored and never stored: the password is
   * set by the verification link. */
  password?: string;
}

/** Sign in with an email and password. The email is a plain string: a malformed
 * address must answer like an unknown one. */
model SiteLoginInput {
  email: string;
  password: string;
}

/** What a login granted (ADR 0020). `session`: the session cookie is set.
 * `challenge`: the password was right but the User has a Second factor; no session
 * exists yet, and the `challenge` goes with a code to `/auth/second-factor`. */
enum SiteLoginOutcome {
  session,
  challenge,
}

model SiteLoginResult {
  outcome: SiteLoginOutcome;

  /** The signed challenge, present when the outcome is `challenge`. It is bound to
   * the User, valid for 5 minutes and works for one successful second step. */
  challenge?: string;
}

/** The second login step: the challenge from the password step and a code */
model SiteLoginSecondFactorInput {
  challenge: string;

  /** A current TOTP code from the authenticator app, or an unused Recovery code */
  code: string;
}

/** Request a password-reset link (always accepted; no account enumeration) */
model SiteForgotPasswordInput {
  email: EmailAddress;
}

/** Ask for the verification link again (always accepted; no account enumeration) */
model SiteResendVerificationInput {
  email: EmailAddress;
}

/** Set a new password using a reset token */
model SiteResetPasswordInput {
  token: string;
  password: string;
}

/** Prove an email address with the token sent to it and set the password */
model SiteVerifyEmailInput {
  token: string;
  password: string;
}

/** Confirm a requested email change using the token sent to the new address */
model SiteConfirmEmailChangeInput {
  token: string;
}

@route("/auth")
@tag("Site Auth")
namespace SiteAuth {
  /** Create an Unverified User (ADR 0030): no password, no session, no Workspace.
   * A verification email with the link to the set-password screen is on its way
   * (202), and 202 is the answer for every address, taken or not (ADR 0031): the
   * owner of a taken address gets a notice by mail instead. With
   * `SPHERICON_REQUIRE_EMAIL_VERIFICATION=false` there is no mail to carry that notice: the
   * request must carry a password (422 otherwise), creates a Verified User with its
   * first Workspace at once (202, no session: the User signs in next), and a taken
   * address answers 409. An Instance that closed sign-up
   * (`SPHERICON_SIGNUP_OPEN=false`, ADR 0042) answers 403 with code `signup_closed`
   * before anything is read or created; accepting an Invitation still works. */
  @useAuth(NoAuth)
  @post
  @route("/register")
  op register(@body body: SiteRegisterInput):
    | AcceptedResponse
    | ForbiddenProblem
    | ConflictProblem
    | UnprocessableEntityProblem
    | TooManyRequestsProblem;

  /** Check the password. A User without a Second factor gets a session (outcome
   * `session`, the JWT cookie set here); a User with one gets outcome `challenge`,
   * a short-lived single-use challenge for the second step and no cookie (ADR
   * 0020). Unknown email and wrong password answer the same 401; failures feed the
   * Login throttle, which answers 429 even for a correct password while its delay
   * runs (ADR 0025). Only a started session resets the throttle's counter. An
   * Unverified User (ADR 0030) answers 403 with code `email_verification_required`
   * before any password check or second-factor challenge, and no session. */
  @useAuth(NoAuth)
  @post
  @route("/login")
  op login(@body body: SiteLoginInput):
    | {
        @statusCode statusCode: 200;

        /** The session cookie, set when the outcome is `session` */
        @header("Set-Cookie") setCookie?: string;

        @body body: SiteLoginResult;
      }
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem;

  /** The second login step of a User with a Second factor: verify the challenge
   * and a TOTP or Recovery code, then start the session. An expired, reused or
   * forged challenge and a wrong code answer the same 401; wrong codes feed the
   * Login throttle of the User's address, which answers 429 even for a correct code
   * while its delay runs (ADR 0020, ADR 0025). */
  @useAuth(NoAuth)
  @post
  @route("/second-factor")
  op secondFactor(@body body: SiteLoginSecondFactorInput):
    | {
        @statusCode statusCode: 200;

        /** The session cookie (always set here; optional only because the
         * response shares its shape with the login operation's) */
        @header("Set-Cookie") setCookie?: string;

        @body body: SiteLoginResult;
      }
    | UnauthorizedProblem
    | TooManyRequestsProblem;

  /** End the session on this browser: clears the session cookie. */
  @useAuth(NoAuth)
  @post
  @route("/logout")
  op logout(): {
    @statusCode statusCode: 204;

    /** The expired session cookie */
    @header("Set-Cookie") setCookie: string;
  };

  /** Request a password-reset link. Always returns 202 regardless of whether
   * the email matches an account and even when the address has used its hourly
   * mail budget, to avoid leaking which addresses exist. Answers 429 over the
   * per-IP limit. */
  @useAuth(NoAuth)
  @post
  @route("/forgot-password")
  op forgotPassword(@body body: SiteForgotPasswordInput):
    | AcceptedResponse
    | TooManyRequestsProblem;

  /** Set a new password from a reset token */
  @useAuth(NoAuth)
  @post
  @route("/reset-password")
  op resetPassword(@body body: SiteResetPasswordInput):
    | OkResponse
    | BadRequestProblem;

  /** Complete a registration from the verification link (the token is the
   * credential): stamps the Verified email, sets the password, ends every earlier
   * session, creates the User's first Workspace, sends the welcome email and starts
   * the session (the cookie is issued after the epoch bump, as login does, ADR 0020).
   * A link is single use and scoped to the address it was sent to. Rate limited like
   * registration. */
  @useAuth(NoAuth)
  @post
  @route("/verify-email")
  op verifyEmail(@body body: SiteVerifyEmailInput):
    | {
        @statusCode statusCode: 200;

        /** The session cookie of the verified User */
        @header("Set-Cookie") setCookie: string;
      }
    | BadRequestProblem
    | TooManyRequestsProblem;

  /** Send the verification link again to an Unverified User. Answers 202 the same
   * way whether the address is unknown, already verified or unverified, so it adds no
   * enumeration; only an Unverified User gets a mail. Rate limited per IP
   * like registration. */
  @useAuth(NoAuth)
  @post
  @route("/resend-verification")
  op resendVerification(@body body: SiteResendVerificationInput):
    | AcceptedResponse
    | TooManyRequestsProblem;

  /** Confirm an email change from the token sent to the new address. Public:
   * the link may be opened from a browser without a session. The change ends
   * every session of the user (ADR 0020); when the request carries a valid
   * session of that same user, it continues under the fresh cookie set in this
   * response. The link alone never starts a session. */
  @useAuth(NoAuth)
  @post
  @route("/confirm-email-change")
  op confirmEmailChange(
    /** The confirming browser's session, if any */
    @cookie("JWT") session?: string,

    @body body: SiteConfirmEmailChangeInput,
  ):
    | {
        @statusCode statusCode: 200;

        /** The confirming session reissued, set when the request carried one */
        @header("Set-Cookie") setCookie?: string;
      }
    | BadRequestProblem;
}
