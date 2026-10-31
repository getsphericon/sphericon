using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** One row of the Audit stream (Enterprise): an Audit entry as a machine reads it. Entries are append-only, so a row is delivered once; a Staff member's entry shows as `staff` with the name "Sphericon staff" and no id, like on every read surface */
model ExportAuditRow {
  /** Unique identifier */
  id: EntityId;

  /** When the change happened */
  occurred_at: Timestamp;

  /** Who did it: `user`, `api_token`, `staff` or `system` */
  actor_kind: string;

  /** The actor's id; absent for the system and for Staff */
  actor_id: string | null;

  /** Display name of the actor when the entry was written */
  actor_name: string | null;

  /** What was done, as `<entity>.<verb>` */
  action: string;

  /** What it was done to */
  target_type: string;

  /** The target's id */
  target_id: string | null;

  /** The target's name when the entry was written; absent for a Contact */
  target_name: string | null;

  /** Before/after of the changed fields as one JSON column; secrets appear as changed only */
  diff: ContactExportJson;

  /** Request id of the change */
  request_id: string | null;

  /** Client address of the change */
  ip: string | null;

  /** Client user agent of the change */
  user_agent: string | null;

  /** Creation timestamp */
  created_at: Timestamp;

  /** Last change timestamp: the cursor column */
  updated_at: Timestamp;
}

@route("/export")
@tag("Export")
namespace Export {
  /**
   * Read the Workspace's Audit entries, oldest first (Enterprise; needs the `export:read`
   * scope and the `audit` feature). Each line of the body is an `ExportAuditRow`; every key
   * is always present. The Audit log list is the human read; this is the machine stream.
   */
  @get
  @route("/audit")
  @requiresScopes(#[ApiTokenScope.exportRead])
  op audit(...ExportPageQuery):
    | ExportPage
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | NotFoundProblem;
}
