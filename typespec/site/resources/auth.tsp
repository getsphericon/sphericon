using TypeSpec.Http;

namespace Sphericon;

/** Register with a name, an email and a password. The User stays Unverified until the
 * address is proven (ADR 0030); the password is trusted only when the verification link
 * is opened in the browser that registered. */
model SiteRegisterInput {
  name: UserName;
  email: EmailAddress;

  /** Required whenever sign-up is open. Stored hashed on the Unverified User; a repeated
   * registration of the address replaces it. */
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

/** Prove an email address with the token sent to it */
model SiteVerifyEmailInput {
  token: string;

  /** Needed only when the link is not opened in the Registration browser: it replaces
   * the password given at registration. Ignored in the Registration browser. */
  password?: string;
}

/** Where a User who proved their address goes next */
model SiteVerifyEmailResult {
  /** The Workspace to open: the one the User joined by its Join domain (ADR 0051), else
   * the first Workspace made for them */
  landingWorkspaceId: WorkspaceId;
}

/** Confirm a requested email change using the token sent to the new address */
model SiteConfirmEmailChangeInput {
  token: string;
}

/** The "this was not me" link of the notice mailed when a Sign-in provider was linked to an account */
model SiteSigninNotMeInput {
  token: string;
}

/** One Sign-in provider an Instance offers (ADR 0048) */
model SiteSigninProvider {
  /** The registry name, the path segment of its `start` and `callback` operations */
  name: string;

  /** What the button says: the provider's own name (`Google`) */
  displayName: string;
}

/** The Sign-in providers this Instance has configured. Empty on an Instance that
 * configured none, which leaves the login and register pages as they are. */
model SiteSigninProviders {
  items: SiteSigninProvider[];
}

/** Why a provider sign-in did not start a session. The callback redirects to `/login` with
 * one of these as the `error` query parameter, and the SPA renders it through i18n. */
enum SiteSigninError {
  /** The person denied consent at the provider */
  denied,

  /** The provider answered with an error, or its answer could not be verified */
  providerError: "provider_error",

  /** The flow's `state` was missing, expired or did not match: a callback this browser did not start */
  invalidState: "invalid_state",

  /** The provider is not configured on this Instance */
  providerUnavailable: "provider_unavailable",

  /** The provider did not vouch for the email address (unverified) */
  emailUnverified: "email_unverified",

  /** The provider's proof of the address is not accepted (a Google address that is neither Gmail nor a hosted domain) */
  emailNotTrusted: "email_not_trusted",

  /** The Instance closed public sign-up (ADR 0042) */
  signupClosed: "signup_closed",

  /** Linking only: the provider account is already linked to another User */
  identityInUse: "identity_in_use",

  /** Accepting an Invitation: the provider's trusted email is not the invited address */
  invitationEmailMismatch: "invitation_email_mismatch",

  /** Accepting an Invitation: the link is not a pending Invitation (unknown, expired or used) */
  invitationInvalid: "invitation_invalid",
}

@route("/auth")
@tag("Site Auth")
namespace SiteAuth {
  /** Create an Unverified User (ADR 0030): no session, no Workspace. A verification
   * email with the link is on its way (202), and 202 is the answer for every address,
   * taken or not (ADR 0031): the owner of a taken address gets a notice by mail
   * instead. The answer always sets the registration cookie (httpOnly, SameSite=Strict,
   * scoped to the auth prefix shared by verify and resend), which binds the link to this browser, so the
   * header does not say whether the address is taken either. The password is required
   * (422 otherwise). With `SPHERICON_REQUIRE_EMAIL_VERIFICATION=false` there is no
   * mail to carry that notice and no link: the request creates a Verified User with its
   * first Workspace at once (202, no session and no cookie: the User signs in next), and
   * a taken address answers 409. An Instance that closed sign-up
   * (`SPHERICON_SIGNUP_OPEN=false`, ADR 0042) answers 403 with code `signup_closed`
   * before anything is read or created; accepting an Invitation still works. */
  @useAuth(NoAuth)
  @post
  @route("/register")
  op register(@body body: SiteRegisterInput):
    | {
        @statusCode statusCode: 202;

        /** The registration cookie, absent when the install does not require verification */
        @header("Set-Cookie") setCookie?: string;
      }
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

  /** The Sign-in providers this Instance offers, for the buttons of the login and register
   * pages. Public; empty when none is configured. */
  @useAuth(NoAuth)
  @get
  @route("/signin-providers")
  op signinProviders(): {
    @statusCode statusCode: 200;
    @body body: SiteSigninProviders;
  };

  /** Start a sign-in with a Sign-in provider: draws the flow's `state`, PKCE verifier and
   * `nonce`, keeps them (with the validated `return_to`) in a short-lived signed cookie
   * (httpOnly, SameSite=Lax, scoped to the signin prefix) and redirects (302) the browser to
   * the provider. `return_to` is honoured only as a relative in-app path; anything else
   * is dropped. 404 for a provider that is not configured. */
  @useAuth(NoAuth)
  @get
  @route("/signin/{provider}/start")
  op signinStart(
    @path provider: string,

    /** A relative in-app path to land on after signing in */
    @query(#{ name: "return_to" }) returnTo?: string,

    /** The token of an Invitation link: the sign-in then accepts that Invitation when the
     * provider's trusted email is the invited address, and an Invitation opens a closed
     * registration for that address (ADR 0042). A mismatch creates nothing. */
    @query invitation?: string,
  ):
    | {
        @statusCode statusCode: 302;

        /** The provider's authorization URL */
        @header("Location") location: string;

        /** The flow cookie */
        @header("Set-Cookie") setCookie: string;
      }
    | NotFoundProblem;

  /** Where the provider sends the browser back (the redirect URI derived from the app URL).
   * Always answers 302: to `return_to` (or `/`) with the session cookie when the sign-in
   * succeeds, and to `/login?error=<code>` (see SiteSigninError) when it does not. A known
   * identity signs in; a new, trusted email creates a Verified User with its first Workspace
   * and no mail. A User with a Second factor gets no session and no cookie: the redirect is
   * to `/login?challenge=<token>`, the challenge of `/auth/second-factor` (ADR 0020), which
   * a TOTP or Recovery code finishes under the Login throttle. Every parameter is optional so that a denied consent (`error` only) or a
   * stray visit reaches the handler and gets the same redirect as any other refusal. */
  @useAuth(NoAuth)
  @get
  @route("/signin/{provider}/callback")
  op signinCallback(
    @path provider: string,

    /** The authorization code */
    @query code?: string,

    /** The flow's `state`, echoed by the provider */
    @query state?: string,

    /** The provider's error code, present when consent was denied or the request failed */
    @query error?: string,

    /** The flow cookie set by `start` */
    @cookie("SIGNIN") flow?: string,

    /** The session cookie: read only when the flow is a link (it names a User), which
     * must be completed by the browser that is signed in as that User */
    @cookie("JWT") session?: string,
  ): {
    @statusCode statusCode: 302;

    /** Where to go next */
    @header("Location") location: string;

    /** The session cookie on success; the expired flow cookie on a refusal */
    @header("Set-Cookie") setCookie?: string;
  };

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

  /** Set a new password from a reset token. A password outside the length rule answers 422
   * with the field error; an invalid or expired token answers 400. */
  @useAuth(NoAuth)
  @post
  @route("/reset-password")
  op resetPassword(@body body: SiteResetPasswordInput):
    | OkResponse
    | BadRequestProblem
    | UnprocessableEntityProblem;

  /** Complete a registration from the verification link (the token is the
   * credential): stamps the Verified email, ends every earlier session, creates the
   * User's first Workspace, sends the welcome email and starts the session (the cookie
   * is issued after the epoch bump, as login does, ADR 0020). When the request carries
   * the registration cookie the link was minted for, the password from registration
   * stays; otherwise the request must carry a password, which replaces it, and without
   * one the answer is 428 and nothing changes, the link stays valid (a mail scanner
   * consumes nothing); a password outside the length rule answers 422, and the link stays
   * valid too. A link is single use and scoped to the address it was sent to.
   * Rate limited like registration. */
  @useAuth(NoAuth)
  @post
  @route("/verify-email")
  op verifyEmail(
    /** The registration cookie of the browser that registered, if any. Its path is the
     * auth prefix shared with resend, and it is SameSite=Strict: not sent on the
     * cross-site click from the mail, but sent on the page's own later request. */
    @cookie("REG") registration?: string,

    @body body: SiteVerifyEmailInput,
  ):
    | {
        @statusCode statusCode: 200;

        /** The session cookie of the verified User */
        @header("Set-Cookie") setCookie: string;

        @body body: SiteVerifyEmailResult;
      }
    | BadRequestProblem
    | PreconditionRequiredProblem
    | UnprocessableEntityProblem
    | TooManyRequestsProblem;

  /** Send the verification link again to an Unverified User. Answers 202 the same
   * way whether the address is unknown, already verified or unverified, so it adds no
   * enumeration; only an Unverified User gets a mail. Rate limited per IP
   * like registration. */
  @useAuth(NoAuth)
  @post
  @route("/resend-verification")
  op resendVerification(
    /** The registration cookie of the browser asking, if any: the new link binds to
     * whatever cookie the caller holds and then signs that browser in without a
     * password */
    @cookie("REG") registration?: string,

    @body body: SiteResendVerificationInput,
  ): AcceptedResponse | TooManyRequestsProblem;

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

  /** Undo a silent link from the notice mailed to the owner (ADR 0048): the token names
   * one Linked identity of the User, which is unlinked, and every session of the User ends.
   * Public: the link may be opened from a browser without a session. A link works once; one
   * whose identity is gone answers 400. */
  @useAuth(NoAuth)
  @post
  @route("/signin-not-me")
  op signinNotMe(@body body: SiteSigninNotMeInput):
    | OkResponse
    | BadRequestProblem;
}
