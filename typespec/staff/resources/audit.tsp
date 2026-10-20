using TypeSpec.Http;

namespace Sphericon;

/** Who performed an audited change, as the Workspace's own team sees it */
enum StaffAuditActorKind {
  user,
  api_token,
  staff,
  service,
  system,
}

/** The actor of an Audit entry */
model StaffAuditActor {
  /** The kind of actor; a staff member carries no id and the name "Sphericon staff"; a service is a Service credential, named as the Staff member who issued it chose */
  kind: StaffAuditActorKind;

  /** The actor's id (a User id or an API token id); absent for the system and a staff member */
  id?: string | null;

  /** Display name snapshot taken when the entry was written */
  name?: string | null;
}

/** What an audited change was done to */
model StaffAuditTarget {
  /** Entity type, e.g. membership */
  type: string;

  /** The target's id */
  id?: string | null;

  /** Name snapshot taken when the entry was written */
  name?: string | null;
}

/** One immutable Audit entry of a Workspace. Secrets are never recorded; they appear as changed only. */
model StaffAuditEntryResource {
  /** Unique identifier */
  id: EntityId;

  /** When the change happened */
  occurredAt: Timestamp;

  /** Who did it */
  actor: StaffAuditActor;

  /** What was done, as `<entity>.<verb>` */
  action: string;

  /** What it was done to */
  target: StaffAuditTarget;

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
model StaffAuditEntryList {
  items: StaffAuditEntryResource[];

  /** Pass as `cursor` for the next page; absent on the last page */
  nextCursor?: string | null;
}

@tag("StaffMember Workspaces")
@route("/workspaces/{workspaceId}/audit-entries")
namespace StaffWorkspaceAudit {
  /** One Workspace's Audit log, newest first. Empty when the instance has no Audit license, since nothing is then recorded. */
  @get
  op list(
    @path workspaceId: WorkspaceId,

    /** Cursor from a previous page */
    @query cursor?: string,

    /** Page size */
    @query limit?: int32 = 25,
  ):
    | StaffAuditEntryList
    | BadRequestProblem
    | UnauthorizedProblem
    | NotFoundProblem;
}
