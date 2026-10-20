using TypeSpec.Http;

namespace Sphericon;

/** A Staff hold: one reversible freeze of a Workspace's outbound sending (ADR 0009) */
model HoldResource {
  /** The key the setter chose; unique per Workspace */
  key: string;

  /** A machine code for the reason */
  reasonCode: string;

  /** The message shown to the Workspace Owner, as given */
  message: string;

  /** Where the Workspace's Owners and Admins can resolve the hold; absent when none was given */
  actionUrl?: string | null;

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

  /** Where the Workspace's Owners and Admins can resolve the hold, shown as a link next to the message:
   * an absolute https URL. Omit it to give none; setting the hold again without it removes a stored one. */
  @maxLength(2048)
  @pattern("^https://")
  actionUrl?: string | null;

  /** Required; in the future and no further than the configured maximum from now */
  expiresAt: Timestamp;
}

/** The outcome of setting a hold */
model HoldSetResult {
  /** False when the hold was already set exactly so and nothing changed, so a retry is visibly harmless */
  changed: boolean;

  /** The hold as it is stored now */
  hold: HoldResource;
}

/** The outcome of clearing a hold */
model HoldClearResult {
  /** False when no such hold was set and nothing changed, so a retry is visibly harmless */
  changed: boolean;
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
  op list(@path workspaceId: WorkspaceId):
    | HoldList
    | UnauthorizedProblem
    | NotFoundProblem
    | TooManyRequestsProblem;

  /** Set a hold under a key, or renew it by setting it again with a later expiry.
   * Idempotent: setting the same hold twice changes nothing and says so (`changed` false);
   * renewing with a different expiry is a change. */
  @route("/{key}")
  @put
  op set(
    @path workspaceId: WorkspaceId,
    @path key: string,
    @body body: HoldInput,
  ):
    | HoldSetResult
    | UnauthorizedProblem
    | NotFoundProblem
    | UnprocessableEntityProblem
    | TooManyRequestsProblem;

  /** Clear a hold by its key. Idempotent: clearing a missing hold succeeds with `changed` false. */
  @route("/{key}")
  @delete
  op clear(@path workspaceId: WorkspaceId, @path key: string):
    | HoldClearResult
    | UnauthorizedProblem
    | NotFoundProblem
    | TooManyRequestsProblem;
}
