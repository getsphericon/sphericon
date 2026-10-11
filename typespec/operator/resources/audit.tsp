using TypeSpec.Http;

namespace Sphericon;

/** Who performed an audited change, as the Workspace's own team sees it */
enum OperatorAuditActorKind {
  user,
  api_token,
  operator,
  system,
}

/** The actor of an Audit entry */
model OperatorAuditActor {
  /** The kind of actor; an operator carries no id and the name "sphericon staff" */
  kind: OperatorAuditActorKind;

  /** The actor's id (a User id or an API token id); absent for the system and an operator */
  id?: string | null;

  /** Display name snapshot taken when the entry was written */
  name?: string | null;
}

/** What an audited change was done to */
model OperatorAuditTarget {
  /** Entity type, e.g. membership */
  type: string;

  /** The target's id */
  id?: string | null;

  /** Name snapshot taken when the entry was written */
  name?: string | null;
}

/** One immutable Audit entry of a Workspace. Secrets are never recorded; they appear as changed only. */
model OperatorAuditEntryResource {
  /** Unique identifier */
  id: EntityId;

  /** When the change happened */
  occurredAt: Timestamp;

  /** Who did it */
  actor: OperatorAuditActor;

  /** What was done, as `<entity>.<verb>` */
  action: string;

  /** What it was done to */
  target: OperatorAuditTarget;

  /** Before/after of the changed fields */
  diff?: Record<unknown> | null;

  /** Request id of the change */
  requestId?: string | null;

  /** Client address of the change */
  ip?: string | null;

  /** Client user agent of the change */
  userAgent?: string | null;
}

/** A page of a Workspace's Audit log, newest first */
model OperatorAuditEntryList {
  items: OperatorAuditEntryResource[];

  /** Pass as `cursor` for the next page; absent on the last page */
  nextCursor?: string | null;
}

@tag("Operator Workspaces")
@route("/workspaces/{workspaceId}/audit-entries")
namespace OperatorWorkspaceAudit {
  /** One Workspace's Audit log, newest first. Empty when the instance has no Audit license, since nothing is then recorded. */
  @get
  op list(
    @path workspaceId: EntityId,

    /** Cursor from a previous page */
    @query cursor?: string,

    /** Page size */
    @query limit?: int32 = 25,
  ):
    | OperatorAuditEntryList
    | BadRequestProblem
    | UnauthorizedProblem
    | NotFoundProblem;
}
