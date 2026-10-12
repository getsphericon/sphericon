using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

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

  /** True only when that email has a Verified User; an address with no User or an
   * Unverified User needs a name and a password to accept */
  hasAccount: boolean;
}

/** Accept an invite. name + password are required unless the invited address has a
 * Verified User (answer 422 otherwise); ignored for a Verified User. */
model SiteAcceptInvitationInput {
  name?: string;
  password?: string;
}

// Public, session-less flow opened from the invite link. Not workspace-scoped.
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

  /** Accept an invite: create or attach the User and create the Membership */
  @useAuth(NoAuth)
  @post
  @route("/{token}/accept")
  op accept(@path token: string, @body body: SiteAcceptInvitationInput):
    | OkResponse
    | NotFoundProblem
    | UnprocessableEntityProblem
    | TooManyRequestsProblem;
}
