using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** Who performed an audited change */
enum SiteAuditActorKind {
  user,
  api_token,
  staff,
  system,
}

/** The actor of an Audit entry */
model SiteAuditActor {
  /** The kind of actor; a staff member is shown to the customer as "Sphericon staff" */
  kind: SiteAuditActorKind;

  /** The actor's id (a User id or an API token id); absent for the system */
  id?: string | null;

  /** Display name snapshot taken when the entry was written */
  name?: string | null;
}

/** What an audited change was done to */
model SiteAuditTarget {
  /** Entity type, e.g. membership */
  type: string;

  /** The target's id */
  id?: string | null;

  /** Name snapshot taken when the entry was written; absent for a Contact */
  name?: string | null;
}

/** One immutable Audit entry (Enterprise) */
model SiteAuditEntryResource {
  /** Unique identifier */
  id: EntityId;

  /** When the change happened */
  occurredAt: Timestamp;

  /** Who did it */
  actor: SiteAuditActor;

  /** What was done, as `<entity>.<verb>` */
  action: string;

  /** What it was done to */
  target: SiteAuditTarget;

  /** Before/after of the changed fields; secrets appear as changed only */
  diff?: Record<unknown> | null;

  /** Request id of the change */
  requestId?: string | null;

  /** Client address of the change */
  ip?: string | null;

  /** Client user agent of the change */
  userAgent?: string | null;
}

/** A page of Audit entries, newest first */
model SiteAuditEntryList {
  items: SiteAuditEntryResource[];

  /** Pass as `cursor` for the next page; absent on the last page */
  nextCursor?: string | null;
}

/** Filters shared by the Audit log list and its CSV export; every one is optional and they combine with AND */
model SiteAuditFilter {
  /** Only entries at or after this time */
  @query from?: Timestamp;

  /** Only entries strictly before this time */
  @query to?: Timestamp;

  /** Only entries by this kind of actor */
  @query actorKind?: SiteAuditActorKind;

  /** Only entries by the actor with this id (a User id or an API token id); ignored for the staff member kind */
  @query actorId?: string;

  /** Only entries with exactly this action, e.g. membership.update */
  @query action?: string;

  /** Only entries about this target type, e.g. integration */
  @query targetType?: string;

  /** Only entries about the target with this id */
  @query targetId?: string;

  /** Only entries from this client address */
  @query ip?: string;

  /** Only entries made under this request id */
  @query requestId?: string;
}

/** A number of days an Audit log is kept: 1 to 3650 */
@minValue(1)
@maxValue(3650)
scalar RetentionDays extends int32;

/** How long the Audit log is kept; the Enterprise advanced-retention control */
model SiteAuditRetention {
  /** Entries older than this many days are pruned; null keeps them forever */
  retentionDays: RetentionDays | null;
}

@route("/workspaces/{workspaceId}/audit-entries")
@tag("Site Audit")
namespace SiteAudit {
  /** List the workspace's Audit log, newest first (Enterprise; owner and admin only) */
  @get
  @requiresPermissions(#[Permission.auditRead])
  op list(
    @path workspaceId: WorkspaceId,
    ...SiteAuditFilter,

    /** Cursor from a previous page */
    @query cursor?: string,

    /** Page size */
    @query
    @minValue(1)
    @maxValue(100)
    limit?: int32 = 25,
  ):
    | SiteAuditEntryList
    | BadRequestProblem
    | PaymentRequiredProblem
    | ForbiddenProblem
    | NotFoundProblem;

  /** Export the Audit log as CSV, newest first, narrowed by the same filter as the list (Enterprise; owner and admin only) */
  @get
  @route("/export")
  @requiresPermissions(#[Permission.auditRead])
  op export(@path workspaceId: WorkspaceId, ...SiteAuditFilter):
    | {
        @header contentType: "text/csv";
        @header contentDisposition: string;
        @body body: string;
      }
    | BadRequestProblem
    | PaymentRequiredProblem
    | ForbiddenProblem
    | NotFoundProblem;

  /** Read the Audit log retention window (Enterprise; owner and admin only) */
  @get
  @route("/retention")
  @requiresPermissions(#[Permission.auditConfigure])
  op getRetention(@path workspaceId: WorkspaceId):
    | SiteAuditRetention
    | BadRequestProblem
    | PaymentRequiredProblem
    | ForbiddenProblem
    | NotFoundProblem;

  /** Set or clear the Audit log retention window; recorded as an Audit entry (Enterprise; owner and admin only) */
  @put
  @route("/retention")
  @requiresPermissions(#[Permission.auditConfigure])
  op setRetention(
    @path workspaceId: WorkspaceId,
    @body body: SiteAuditRetention,
  ):
    | SiteAuditRetention
    | BadRequestProblem
    | PaymentRequiredProblem
    | ForbiddenProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;
}
