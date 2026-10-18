using TypeSpec.Http;

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
  /** The kind of actor; a staff member is shown to the customer as "sphericon staff" */
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

  /** True when the change set, renewed or lifted a billing hold (reason code `non_payment` or `free_plan_cap`): shown as billing activity */
  billing: boolean;

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

/** How long the Audit log is kept; the Enterprise advanced-retention control */
model SiteAuditRetention {
  /** Entries older than this many days are pruned; null keeps them forever */
  retentionDays: int32 | null;
}

@route("/workspaces/{slug}/audit-entries")
@tag("Site Audit")
namespace SiteAudit {
  /** List the workspace's Audit log, newest first (Enterprise; owner and admin only) */
  @get
  op list(
    @path slug: string,
    ...SiteAuditFilter,

    /** Cursor from a previous page */
    @query cursor?: string,

    /** Page size */
    @query limit?: int32 = 25,
  ):
    | SiteAuditEntryList
    | BadRequestProblem
    | PaymentRequiredProblem
    | ForbiddenProblem
    | NotFoundProblem;

  /** Export the Audit log as CSV, newest first, narrowed by the same filter as the list (Enterprise; owner and admin only) */
  @get
  @route("/export")
  op export(@path slug: string, ...SiteAuditFilter):
    | {
        @header contentType: "text/csv";
        @header contentDisposition: string;
        @body body: string;
      }
    | PaymentRequiredProblem
    | ForbiddenProblem
    | NotFoundProblem;

  /** Read the Audit log retention window (Enterprise; owner and admin only) */
  @get
  @route("/retention")
  op getRetention(@path slug: string):
    | SiteAuditRetention
    | PaymentRequiredProblem
    | ForbiddenProblem
    | NotFoundProblem;

  /** Set or clear the Audit log retention window; recorded as an Audit entry (Enterprise; owner and admin only) */
  @put
  @route("/retention")
  op setRetention(@path slug: string, @body body: SiteAuditRetention):
    | SiteAuditRetention
    | PaymentRequiredProblem
    | ForbiddenProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;
}
