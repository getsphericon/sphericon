using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** Roles that may be invited. owner is never invited — it is transferred. */
enum SiteInvitableRole {
  admin,
  member,
}

/** A pending invitation of an email address to join the workspace */
@resource("invitations")
@segment("invitations")
@parentResource(SiteWorkspaceResource)
model SiteInvitationResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** The invited email address */
  email: EmailAddress;

  /** The role the invitee will receive on accept */
  role: SiteInvitableRole;

  /** When the invitation link expires */
  expiresAt: Timestamp;

  /** Email of the User who sent the invite, if still known */
  invitedByEmail?: string | null;

  /** Creation timestamp */
  createdAt: Timestamp;
}

/** Invite an email address to the workspace */
model SiteCreateInvitationInput {
  email: EmailAddress;
  role: SiteInvitableRole;
}

/** Invite creation result. `inviteUrl` is the one-time accept link — shown so the
 * UI can offer "copy link"; an email is also sent best-effort. */
model SiteCreateInvitationResponse {
  inviteUrl: string;
  resource: SiteInvitationResource;
}

// Bespoke (not WorkspaceCrud): create returns the one-time invite URL; no
// single-item GET/PUT. Nested under the workspace via @parentResource.
@tag("Site Invitations")
namespace SiteInvitations {
  /** List pending (unaccepted) invitations for the workspace */
  @listsResource(SiteInvitationResource)
  @get
  @autoRoute
  @extension("x-required-permissions", #[])
  op list(...ResourceCollectionParameters<SiteInvitationResource>):
    | {
        @statusCode statusCode: 200;
        @body body: SiteInvitationResource[];
      }
    | NotFoundProblem;

  /** Invite an email address; the accept link is returned once */
  @createsResource(SiteInvitationResource)
  @post
  @autoRoute
  @extension("x-required-permissions", #[Permission.membersManage])
  op create(
    ...ResourceCollectionParameters<SiteInvitationResource>,
    @body body: SiteCreateInvitationInput,
  ):
    | {
        @statusCode statusCode: 201;
        @body body: SiteCreateInvitationResponse;
      }
    | NotFoundProblem
    | ForbiddenProblem
    | ConflictProblem
    | UnprocessableEntityProblem;

  /** Revoke a pending invitation */
  @deletesResource(SiteInvitationResource)
  @delete
  @autoRoute
  @extension("x-required-permissions", #[Permission.membersManage])
  op delete(...ResourceParameters<SiteInvitationResource>):
    | NoContentResponse
    | NotFoundProblem
    | ForbiddenProblem;
}

/** What an invite link reveals before acceptance, for rendering the accept page */
model SiteInvitationLookupResult {
  /** The workspace the invitee is joining */
  workspaceName: string;

  /** The address the invite was sent to */
  email: EmailAddress;
}

/** Accept an invite. For an address with no User or an Unverified User, name + password
 * are required (422 otherwise), and the Sign-up items are answered as on the sign-up form. For a
 * Verified User the body is not read: the session of that same User is the proof, and consent
 * follows the User, so nothing is asked again (ADR 0062). */
model SiteAcceptInvitationInput {
  name?: string;
  password?: string;

  /** The answers to the Sign-up items in effect, read only when the accept sets up the account
   * (ADR 0062). Every required acceptance must be answered ticked at its current version, or the
   * field `signupItems.<key>` is refused; an optional consent left out is not recorded. Each
   * answered item is recorded as accepted or declined, and each notice in effect as shown, with
   * the source Invitation. Absent with no items in effect. */
  signupItems?: SiteSignupItemAnswer[];
}

// Opened from the invite link. Not workspace-scoped. Lookup is session-less; accept
// takes an optional session cookie, required (and matched) only for a Verified User.
@route("/invitations")
@tag("Site Invitations")
namespace SitePublicInvitations {
  /** Look up a pending invite by its token to render the accept page */
  @useAuth(NoAuth)
  @get
  @route("/{token}")
  op lookup(@path token: string):
    | {
        @statusCode statusCode: 200;
        @body body: SiteInvitationLookupResult;
      }
    | NotFoundProblem;

  /** Accept an invite: create or complete the User, or attach the signed-in Verified
   * User, and create the Membership. 401 without a session and 403 for another User's
   * session when the address has a Verified User. */
  @useAuth(ApiKeyAuth<ApiKeyLocation.cookie, "JWT"> | NoAuth)
  @post
  @route("/{token}/accept")
  op accept(@path token: string, @body body: SiteAcceptInvitationInput):
    | OkResponse
    | UnauthorizedProblem
    | ForbiddenProblem
    | NotFoundProblem
    | UnprocessableEntityProblem
    | TooManyRequestsProblem;
}
