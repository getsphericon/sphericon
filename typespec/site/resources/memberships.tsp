using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

namespace Sphericon;

/** A User's permission level in a Workspace */
enum SiteMembershipRole {
  owner,
  admin,
  member,
}

/** How a member got in */
enum SiteMembershipSource {
  creator,
  invitation,
  domain,
}

/** A Membership — the join granting a User access to the workspace with a Role */
@resource("memberships")
@segment("memberships")
@parentResource(SiteWorkspaceResource)
model SiteMembershipResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** The member's User id */
  userId: EntityId;

  /** The member's email */
  email: EmailAddress;

  /** The member's display name */
  name: string;

  /** The member's role in this workspace */
  role: SiteMembershipRole;

  /** How the member got in; absent for a member from before it was recorded */
  source?: SiteMembershipSource;

  /** When the member joined */
  createdAt: Timestamp;
}

/** Change a member's role. Only an owner may grant/transfer the owner role. */
model SiteUpdateMembershipInput {
  role: SiteMembershipRole;
}

// Bespoke (not WorkspaceCrud): members are created by accepting an Invitation,
// never via POST; only role change (PUT) and removal (DELETE) are exposed.
@tag("Site Memberships")
namespace SiteMemberships {
  /** List the workspace's members */
  @listsResource(SiteMembershipResource)
  @get
  @autoRoute
  op list(...ResourceCollectionParameters<SiteMembershipResource>):
    | {
        @statusCode statusCode: 200;
        @body body: SiteMembershipResource[];
      }
    | NotFoundProblem;

  /** Change a member's role (owner/admin only; owner-only to grant owner) */
  @updatesResource(SiteMembershipResource)
  @put
  @autoRoute
  op update(
    ...ResourceParameters<SiteMembershipResource>,
    @body body: SiteUpdateMembershipInput,
  ):
    | SiteMembershipResource
    | NotFoundProblem
    | ForbiddenProblem
    | UnprocessableEntityProblem;

  /** Remove a member (owner/admin only; the last owner cannot be removed) */
  @deletesResource(SiteMembershipResource)
  @delete
  @autoRoute
  op delete(...ResourceParameters<SiteMembershipResource>):
    | NoContentResponse
    | NotFoundProblem
    | ForbiddenProblem
    | UnprocessableEntityProblem;

  /** Reset the member's Second factor (owner/admin only; owner-only for an owner):
   * clears the factor and its Recovery codes and ends every session of theirs. The
   * acting session is untouched. 422 when the member has no Second factor or is the
   * caller (who disables their own with a password and a code). */
  @action("reset-second-factor")
  @autoRoute
  op resetSecondFactor(...ResourceParameters<SiteMembershipResource>):
    | NoContentResponse
    | NotFoundProblem
    | ForbiddenProblem
    | UnprocessableEntityProblem;
}
