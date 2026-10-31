using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** A Tag: a presence-only, workspace-scoped label on Contacts (auto-created on first use). */
model SiteTagResource {
  /** Unique identifier */
  id: EntityId;

  /** The label; unique per workspace */
  name: string;

  /** Creation timestamp */
  createdAt: Timestamp;
}

@route("/workspaces/{workspaceId}")
@tag("Site Tags")
namespace SiteTags {
  /** List the workspace's tags (feeds the segment builder) */
  @route("/tags")
  @get
  @requiresPermissions(#[])
  op list(@path workspaceId: WorkspaceId, ...PageQuery):
    | Page<SiteTagResource>
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** List the tags a contact has */
  @route("/contacts/{contactId}/tags")
  @get
  @requiresPermissions(#[])
  op listForContact(
    @path workspaceId: WorkspaceId,
    @path contactId: EntityId,
    ...PageQuery,
  ):
    | Page<SiteTagResource>
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Apply a tag to a contact, creating the tag on first use. Idempotent. */
  @route("/contacts/{contactId}/tags")
  @post
  @requiresPermissions(#[])
  op apply(
    @path workspaceId: WorkspaceId,
    @path contactId: EntityId,
    @body body: ApplyTagInput,
  ):
    | SiteTagResource
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Delete a tag from the catalogue, unless a segment rule or an automation step names it */
  @route("/tags/{id}")
  @delete
  @requiresPermissions(#[])
  op delete(@path workspaceId: WorkspaceId, @path id: EntityId):
    | NoContentResponse
    | BadRequestProblem
    | NotFoundProblem
    | InUseProblem;

  /** Remove a tag from a contact. Idempotent; the tag stays in the catalogue. */
  @route("/contacts/{contactId}/tags/{name}")
  @delete
  @requiresPermissions(#[])
  op remove(
    @path workspaceId: WorkspaceId,
    @path contactId: EntityId,
    @path name: string,
  ): NoContentResponse | BadRequestProblem | NotFoundProblem;
}
