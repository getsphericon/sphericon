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

/** A Workspace the User is the only Owner of, which blocks User deletion */
model SiteSoleOwnedWorkspace {
  /** The Workspace id */
  id: WorkspaceId;

  /** Display name */
  name: string;
}

/** Confirmation of User deletion (ADR 0050): the User's own email typed in, their password
 * (a User with no password sends the token of the mailed link instead) and, when a Second
 * factor is enrolled, a current code. */
model SiteDeleteUserInput {
  /** The User's own email address, typed in */
  email: EmailAddress;

  /** Current password; required of a User who has one */
  currentPassword?: string;

  /** The token of the one-time link mailed to a User with no password; required of that User */
  token?: string;

  /** A current TOTP code, or an unused Recovery code; required when a Second factor is enrolled */
  code?: string;
}

/** The request for the one-time deletion link of a User with no password (ADR 0050) */
model SiteDeleteUserLinkInput {
  /** The User's own email address, typed in */
  email: EmailAddress;
}

/** The body of a refused User deletion: ProblemDetails plus, when the User is the only Owner
 * of Workspaces, the list of them (empty when the typed email did not match) */
model SiteDeleteUserProblemDetails {
  ...ProblemDetails;

  /** The live Workspaces the User is the only Owner of; each is handed over or deleted first */
  workspaces: SiteSoleOwnedWorkspace[];
}

/** RFC 7807 validation response of User deletion */
@error
model SiteDeleteUserRefusedProblem is Response<422> {
  @header contentType: "application/problem+json";
  @body body: SiteDeleteUserProblemDetails;
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

  /** Delete the User, immediately and irreversibly (User deletion, ADR 0050): every Membership,
   * Linked identity, the Second factor and the Recovery codes go, every session ends (the
   * cookie is cleared here) and one email tells the address. Needs the User's own email, the
   * password (a User with no password, signed in only through a Sign-in provider, sends the
   * token of the link mailed by `deleteUserLink` instead: 400 when it is expired, used, or
   * was issued to another User) and, with a Second factor, a current code. 403 on a wrong
   * password or code (both feed the Login throttle: 429 while its delay runs). 422 when the typed email is not the
   * User's, or while the User is the only Owner of a Workspace that is not in its deletion
   * window: the Workspaces are listed and nothing is deleted. A Workspace in its deletion
   * window does not block. */
  @post
  @route("/delete")
  op deleteUser(@body body: SiteDeleteUserInput):
    | {
        @statusCode statusCode: 204;

        /** The expired session cookie */
        @header("Set-Cookie") setCookie: string;
      }
    | BadRequestProblem
    | ForbiddenProblem
    | SiteDeleteUserRefusedProblem
    | TooManyRequestsProblem;

  /** Mail the one-time deletion link to a User with no password (ADR 0050): it goes to the
   * User's Verified email, is bound to the User (not the address), expires after an hour and
   * works once. 202 whether or not a mail is due (a mail flood is throttled silently). 422
   * when the typed email is not the User's, or when the User has a password and confirms with
   * it instead. */
  @post
  @route("/delete/link")
  op deleteUserLink(@body body: SiteDeleteUserLinkInput):
    | AcceptedResponse
    | SiteDeleteUserRefusedProblem;
}
