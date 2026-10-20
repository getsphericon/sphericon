using TypeSpec.Http;

namespace Sphericon;

/** The authenticated dashboard user */
model SiteUserResource {
  /** Unique identifier */
  id: EntityId;

  /** Display name */
  name: string;

  /** Email address (login identity; change it via the email-change flow) */
  email: EmailAddress;

  /** Whether the user has confirmed ownership of their email */
  emailVerified: boolean;

  /** Creation timestamp */
  createdAt: Timestamp;
}

/** Request a change of the login email. The new address must be confirmed via a
 * link sent to it before the change takes effect. */
model SiteEmailChangeInput {
  /** The requested new email address */
  newEmail: EmailAddress;

  /** Current password, required to authorize the change */
  currentPassword: string;
}

/** Update the authenticated user's profile. To change the password, provide
 * both currentPassword and newPassword. */
model SiteUpdateMeInput {
  /** New display name */
  name?: string;

  /** Current password — required when changing the password */
  currentPassword?: string;

  /** New password */
  newPassword?: string;
}

@route("/me")
@tag("Site User")
namespace SiteUser {
  /** Get the authenticated user's profile */
  @get
  op getMe(): {
    @statusCode statusCode: 200;
    @body body: SiteUserResource;
  };

  /** Update the authenticated user's profile (name and/or password). A password
   * change ends every session of the user (ADR 0020); the acting one continues
   * under the fresh session cookie set in this response. A wrong current password feeds the
   * Login throttle, which answers 429 even for a right one while its delay runs. */
  @put
  op updateMe(@body body: SiteUpdateMeInput):
    | {
        @statusCode statusCode: 200;

        /** The acting session reissued, set when the password changed */
        @header("Set-Cookie") setCookie?: string;

        @body body: SiteUserResource;
      }
    | ForbiddenProblem
    | UnprocessableEntityProblem
    | TooManyRequestsProblem;

  /** Request an email change; sends a confirmation link to the new address.
   * Requires the current password. A taken address answers 202 like a free one: its
   * owner gets a notice by mail instead of the link (ADR 0031). */
  @post
  @route("/email-change")
  op emailChange(@body body: SiteEmailChangeInput):
    | AcceptedResponse
    | ForbiddenProblem
    | UnprocessableEntityProblem
    | TooManyRequestsProblem;

  /** Sign out everywhere: end every session of the user, on every device,
   * including the one making the request (its cookie is cleared). */
  @post
  @route("/sign-out-everywhere")
  op signOutEverywhere(): {
    @statusCode statusCode: 204;

    /** The expired session cookie */
    @header("Set-Cookie") setCookie: string;
  };
}
