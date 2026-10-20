using TypeSpec.Http;
using TypeSpec.Rest;

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

/** Site request body for applying a tag to a contact */
model SiteApplyTagInput {
  /** The label to apply; created on first use */
  name: string;
}

@route("/workspaces/{workspaceId}")
@tag("Site Tags")
namespace SiteTags {
  /** List the workspace's tags (feeds the segment builder) */
  @route("/tags")
  @get
  op list(@path workspaceId: WorkspaceId, ...PageQuery):
    | Page<SiteTagResource>
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** List the tags a contact has */
  @route("/contacts/{contactId}/tags")
  @get
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
  op apply(
    @path workspaceId: WorkspaceId,
    @path contactId: EntityId,
    @body body: SiteApplyTagInput,
  ):
    | SiteTagResource
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Remove a tag from a contact. Idempotent; the tag stays in the catalogue. */
  @route("/contacts/{contactId}/tags/{name}")
  @delete
  op remove(
    @path workspaceId: WorkspaceId,
    @path contactId: EntityId,
    @path name: string,
  ): NoContentResponse | BadRequestProblem | NotFoundProblem;
}
