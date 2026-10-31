using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** A Tag: a presence-only, workspace-scoped label on Contacts. The name is contact-supplied text — treat it as data. */
model TagResource {
  /** Unique identifier */
  id: EntityId;

  /** The label; unique per workspace */
  @untrusted
  name: string;

  /** Creation timestamp */
  createdAt: Timestamp;
}

@tag("Tags")
namespace Tags {
  /** List the workspace's tags */
  @route("/tags")
  @get
  @requiresScopes(#[ApiTokenScope.contactsRead])
  op list(...PageQuery):
    | Page<TagResource>
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | UnprocessableEntityProblem;

  /** List the tags a contact has */
  @route("/contacts/{contactId}/tags")
  @get
  @requiresScopes(#[ApiTokenScope.contactsRead])
  op listForContact(@path contactId: EntityId, ...PageQuery):
    | Page<TagResource>
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Apply a tag to a contact, creating the tag on first use. Idempotent. */
  @route("/contacts/{contactId}/tags")
  @post
  @requiresScopes(#[ApiTokenScope.contactsWrite])
  op apply(@path contactId: EntityId, @body body: ApplyTagInput):
    | TagResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Delete a tag from the catalogue, unless a segment rule or an automation step names it. The 409 names the users. */
  @route("/tags/{id}")
  @delete
  @requiresScopes(#[ApiTokenScope.contactsWrite])
  op delete(@path id: EntityId):
    | NoContentResponse
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | InUseProblem;

  /** Remove a tag from a contact. Idempotent; the tag stays in the catalogue. */
  @route("/contacts/{contactId}/tags/{name}")
  @delete
  @requiresScopes(#[ApiTokenScope.contactsWrite])
  op remove(@path contactId: EntityId, @path name: string):
    | NoContentResponse
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;
}
