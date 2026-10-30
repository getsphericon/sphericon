using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** A Connection: the access a User gave an application (an MCP client) to the Workspace by
 * consenting to it (ADR 0061). It belongs to that User's Membership, can do no more than the
 * User's Role allows and ends with the Membership. Connections are not API tokens and are not
 * listed with them. */
@resource("connections")
@segment("connections")
@parentResource(SiteWorkspaceResource)
model SiteConnectionResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** The application's name, as it registered itself (untrusted: show as text) */
  name: string;

  /** Granted scopes */
  scopes: ApiTokenScope[];

  /** The User the Connection belongs to */
  userId: EntityId;

  /** That User's display name */
  userName: string;

  /** That User's email */
  userEmail: EmailAddress;

  /** When the User approved the application */
  createdAt: Timestamp;
}

// Bespoke (not WorkspaceCrud): a Connection is created by consenting on the OAuth screen, never
// via POST, and is only listed and revoked here. Each User sees and revokes their own;
// members:manage sees and revokes anyone's (a Policy of the module, not a declaration).
@tag("Site Connections")
namespace SiteConnections {
  /** List the Connections the caller may see: their own, or every member's with members:manage */
  @listsResource(SiteConnectionResource)
  @get
  @autoRoute
  @extension("x-required-permissions", #[])
  op list(...ResourceCollectionParameters<SiteConnectionResource>):
    | {
        @statusCode statusCode: 200;
        @body body: SiteConnectionResource[];
      }
    | NotFoundProblem;

  /** Revoke a Connection: the caller's own, or anyone's with members:manage */
  @deletesResource(SiteConnectionResource)
  @delete
  @autoRoute
  @extension("x-required-permissions", #[])
  op delete(...ResourceParameters<SiteConnectionResource>):
    | NoContentResponse
    | NotFoundProblem;
}
