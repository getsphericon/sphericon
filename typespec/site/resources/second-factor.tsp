using TypeSpec.Http;

namespace Sphericon;

/** The authenticated User's Second factor (ADR 0020). Recovery codes are never
 * readable here: only how many are left. */
model SiteSecondFactorStatus {
  /** Whether a confirmed TOTP Second factor is active */
  enabled: boolean;

  /** Whether an enrollment was started and still awaits its confirmation code */
  pending: boolean;

  /** Unused Recovery codes left (0 without a Second factor) */
  recoveryCodesRemaining: int32;

  /** Whether the User has a password. Without one, the steps that ask for the password
   * accept a fresh sign-in with a Sign-in provider instead. */
  passwordSet: boolean;

  /** Whether the session holds a fresh proof that stands in for the password: a Sign-in
   * provider login or a Passkey check of the last few minutes (a User with a password always
   * proves the password). When a User with no password sees false, the next step is to sign
   * in with a provider again or to check a Passkey. */
  identityProofFresh: boolean;
}

/** A pending enrollment: the TOTP secret to add to an authenticator app */
model SiteSecondFactorEnrollment {
  /** The secret as a base32 key, for typing it into the app */
  secret: string;

  /** The otpauth:// URI the QR code encodes */
  otpauthUri: string;

  /** The QR code of the otpauth URI, as a PNG data URI */
  qrCode: string;
}

/** Proof of the User's password (or a fresh provider login or Passkey check), for starting an enrollment */
model SiteSecondFactorStartInput {
  /** Current password. Left out by a User with no password, who proves a provider or Passkey
   * sign-in of the last few minutes instead (ADR 0048). */
  currentPassword?: string;
}

/** Proof of the User's password (or a fresh provider login or Passkey check) and a code from the authenticator app */
model SiteSecondFactorConfirmInput {
  /** Current password. Left out by a User with no password, who proves a provider or Passkey
   * sign-in of the last few minutes instead (ADR 0048). */
  currentPassword?: string;

  /** The current 6-digit TOTP code */
  code: string;
}

/** A fresh set of Recovery codes. They are shown only in this response. */
model SiteRecoveryCodes {
  /** Single-use codes, each one usable in place of a TOTP code */
  codes: string[];
}

/** Proof of the User's password (or a fresh provider login or Passkey check), for regenerating Recovery codes */
model SiteRecoveryCodesInput {
  /** Current password. Left out by a User with no password, who proves a provider or Passkey
   * sign-in of the last few minutes instead (ADR 0048). */
  currentPassword?: string;
}

/** Proof of password (or a fresh provider login or Passkey check) and possession, for disabling the Second factor */
model SiteSecondFactorDisableInput {
  /** Current password. Left out by a User with no password, who proves a provider or Passkey
   * sign-in of the last few minutes instead (ADR 0048). */
  currentPassword?: string;

  /** A current TOTP code, or an unused Recovery code */
  code: string;
}

@route("/me/second-factor")
@tag("Site Second Factor")
namespace SiteSecondFactor {
  /** The authenticated User's Second factor status */
  @get
  op getStatus(): {
    @statusCode statusCode: 200;
    @body body: SiteSecondFactorStatus;
  };

  /** Start enrolling a TOTP Second factor, proving the password: creates a pending
   * secret (replacing an earlier pending one). It counts as a Second factor only
   * once confirmed. 403 on a wrong password (or, for a User with no password, no provider sign-in or Passkey check in the last few minutes), 409 when a Second factor is already
   * active. A wrong password feeds the Login throttle, which answers 429 even for a
   * correct one while its delay runs (ADR 0025). */
  @post
  @route("/enrollment")
  op startEnrollment(@body body: SiteSecondFactorStartInput):
    | {
        @statusCode statusCode: 200;
        @body body: SiteSecondFactorEnrollment;
      }
    | ForbiddenProblem
    | ConflictProblem
    | TooManyRequestsProblem;

  /** Confirm the pending enrollment with the password and a code from the app. On
   * success the Second factor is active, every other session ends and the acting
   * one continues under the cookie set here; the Recovery codes are returned once.
   * 403 on a wrong password (or, for a User with no password, no provider sign-in or Passkey check in the last few minutes) (it feeds the Login throttle: 429 while its delay
   * runs), 422 on a wrong code, 409 without a pending enrollment. */
  @post
  @route("/enrollment/confirm")
  op confirmEnrollment(@body body: SiteSecondFactorConfirmInput):
    | {
        @statusCode statusCode: 200;

        /** The acting session reissued under the new session epoch */
        @header("Set-Cookie") setCookie: string;

        @body body: SiteRecoveryCodes;
      }
    | ForbiddenProblem
    | ConflictProblem
    | UnprocessableEntityProblem
    | TooManyRequestsProblem;

  /** Replace the Recovery codes with a fresh set (the previous set stops working).
   * Every other session ends; the acting one continues under the cookie set here.
   * 403 on a wrong password (or, for a User with no password, no provider sign-in or Passkey check in the last few minutes) (it feeds the Login throttle: 429 while its delay
   * runs), 409 without an active Second factor. */
  @post
  @route("/recovery-codes")
  op regenerateRecoveryCodes(@body body: SiteRecoveryCodesInput):
    | {
        @statusCode statusCode: 200;

        /** The acting session reissued under the new session epoch */
        @header("Set-Cookie") setCookie: string;

        @body body: SiteRecoveryCodes;
      }
    | ForbiddenProblem
    | ConflictProblem
    | TooManyRequestsProblem;

  /** Disable the Second factor, proving the password and a current code. Every
   * other session ends; the acting one continues under the cookie set here. 403 on
   * a wrong password (it feeds the Login throttle: 429 while its delay runs), 422
   * on a wrong code, 409 without an active Second factor. */
  @post
  @route("/disable")
  op disable(@body body: SiteSecondFactorDisableInput):
    | {
        @statusCode statusCode: 204;

        /** The acting session reissued under the new session epoch */
        @header("Set-Cookie") setCookie: string;
      }
    | ForbiddenProblem
    | ConflictProblem
    | UnprocessableEntityProblem
    | TooManyRequestsProblem;
}
