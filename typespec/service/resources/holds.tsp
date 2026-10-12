using TypeSpec.Http;

namespace Sphericon;

/** An Operator hold: one reversible freeze of a Workspace's outbound sending (ADR 0009) */
model HoldResource {
  /** The key the setter chose; unique per Workspace */
  key: string;

  /** A machine code for the reason */
  reasonCode: string;

  /** The message shown to the Workspace Owner, as given */
  message: string;

  /** When the hold stops freezing sending unless its setter renews it */
  expiresAt: Timestamp;

  /** The name of the Service credential that last set it */
  setBy: string;

  /** When the hold was first set */
  createdAt: Timestamp;

  /** When the hold was last set or renewed */
  updatedAt: Timestamp;
}

/** Setting or renewing a hold */
model HoldInput {
  /** A machine code for the reason: lowercase letters, digits and underscores */
  @minLength(1)
  @maxLength(64)
  @pattern("^[a-z0-9_]+$")
  reasonCode: string;

  /** The message shown to the Workspace Owner, as given */
  @minLength(1)
  @maxLength(1000)
  message: string;

  /** Required; in the future and no further than the configured maximum from now */
  expiresAt: Timestamp;
}

/** The active holds of a Workspace */
model HoldList {
  items: HoldResource[];
}

@tag("Service Holds")
@route("/workspaces/{workspaceId}/holds")
namespace ServiceHolds {
  /** The holds of a Workspace that are active now; a lapsed one is not listed */
  @get
  op list(@path workspaceId: EntityId):
    | HoldList
    | UnauthorizedProblem
    | NotFoundProblem
    | TooManyRequestsProblem;

  /** Set a hold under a key, or renew it by setting it again with a later expiry.
   * Idempotent: setting the same hold twice changes nothing. */
  @route("/{key}")
  @put
  op set(
    @path workspaceId: EntityId,
    @path key: string,
    @body body: HoldInput,
  ):
    | HoldResource
    | UnauthorizedProblem
    | NotFoundProblem
    | UnprocessableEntityProblem
    | TooManyRequestsProblem;

  /** Clear a hold by its key. Idempotent: clearing a missing hold succeeds. */
  @route("/{key}")
  @delete
  op clear(@path workspaceId: EntityId, @path key: string):
    | NoContentResponse
    | UnauthorizedProblem
    | NotFoundProblem
    | TooManyRequestsProblem;
}
