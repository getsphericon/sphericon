using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** A Personal token: a User's own bearer credential for the external API in this Workspace, for
 * the scripts and tools they run (ADR 0061: a Connection without the application). It belongs to
 * that User's Membership, can do no more than the User's current Role allows and ends with the
 * Membership. Its secret is shown only once, at creation. Personal tokens are neither the
 * Workspace's API tokens nor Connections and are listed with neither. */
@resource("personal-tokens")
@segment("personal-tokens")
@parentResource(SiteWorkspaceResource)
model SitePersonalTokenResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** Human-readable label */
  name: string;

  /** Public prefix (the secret is never returned after creation) */
  prefix: string;

  /** Granted scopes */
  scopes: PersonalScope[];

  /** Expiry, if set */
  expiresAt?: Timestamp | null;

  /** The User the Personal token belongs to */
  userId: EntityId;

  /** That User's display name */
  userName: string;

  /** That User's email */
  userEmail: EmailAddress;

  /** Creation timestamp */
  createdAt: Timestamp;
}

/** Mint a Personal token of the caller's own */
model SiteCreatePersonalTokenInput {
  name: string;

  /** Each one a scope the caller's Role may grant (`grantableScopes`) */
  scopes: PersonalScope[];

  expiresAt?: Timestamp | null;
}

/** Personal token creation result: `token` is the full secret, shown only once */
model SiteCreatePersonalTokenResponse {
  token: string;
  resource: SitePersonalTokenResource;
}

// Bespoke (not WorkspaceCrud): any member mints one of their own, create returns the one-time
// secret, and there is no GET or PUT of one. Each User sees and revokes their own; members:manage
// sees and revokes anyone's (the Policy of a Connection, in the module, not a declaration).
@tag("Site Personal Tokens")
namespace SitePersonalTokens {
  /** List the active Personal tokens the caller may see: their own, or every member's with
   * members:manage */
  @listsResource(SitePersonalTokenResource)
  @get
  @autoRoute
  @extension("x-required-permissions", #[])
  op list(...ResourceCollectionParameters<SitePersonalTokenResource>):
    | {
        @statusCode statusCode: 200;
        @body body: SitePersonalTokenResource[];
      }
    | NotFoundProblem;

  /** Mint a Personal token of the caller's own; the full secret is returned once. Any member may
   * mint one, with the scopes their Role may grant: another is refused (403 permission_required).
   * A scope that is no `PersonalScope` (a token-only scope such as `tokens:write`) is not a request
   * this operation decodes (400). */
  @createsResource(SitePersonalTokenResource)
  @post
  @autoRoute
  @extension("x-required-permissions", #[])
  op create(
    ...ResourceCollectionParameters<SitePersonalTokenResource>,
    @body body: SiteCreatePersonalTokenInput,
  ):
    | {
        @statusCode statusCode: 201;
        @body body: SiteCreatePersonalTokenResponse;
      }
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Revoke a Personal token: the caller's own, or anyone's with members:manage */
  @deletesResource(SitePersonalTokenResource)
  @delete
  @autoRoute
  @extension("x-required-permissions", #[])
  op delete(...ResourceParameters<SitePersonalTokenResource>):
    | NoContentResponse
    | NotFoundProblem;

  /** The scopes the caller's Role may grant to a Personal token in the Workspace, in the
   * vocabulary's order: the mint form offers these and no other. */
  @get
  @route("/workspaces/{workspaceId}/personal-token-scopes")
  @extension("x-required-permissions", #[])
  op grantableScopes(@path workspaceId: WorkspaceId):
    | {
        @statusCode statusCode: 200;
        @body body: PersonalScope[];
      }
    | NotFoundProblem;
}
