import "./audit.tsp";
import "./cursor-pages.tsp";

using TypeSpec.Http;

// A Workspace's Audit log as /site and /api show and filter it. The Staff console names one more
// kind of actor and declares its own.

namespace Sphericon;

/** Who performed an audited change */
enum AuditActorKind {
  user,
  api_token,
  staff,
  system,
}

/** The actor of an Audit entry */
model AuditActor {
  /** The kind of actor; a staff member is shown to the customer as "Sphericon staff" */
  kind: AuditActorKind;

  /** The actor's id (a User id or an API token id); absent for the system */
  id?: string | null;

  /** Display name snapshot taken when the entry was written */
  name?: string | null;
}

/** One immutable Audit entry (Enterprise) */
model AuditEntryResource is AuditEntryOf<AuditActor>;

/** A page of Audit entries, newest first */
model AuditEntryList is CursorPage<AuditEntryResource>;

/** Filters of an Audit log read; every one is optional and they combine with AND */
model AuditFilter {
  /** Only entries at or after this time */
  @query from?: Timestamp;

  /** Only entries strictly before this time */
  @query to?: Timestamp;

  /** Only entries by this kind of actor */
  @query actorKind?: AuditActorKind;

  /** Only entries by the actor with this id (a User id or an API token id); ignored for the staff member kind */
  @query actorId?: string;

  /** Only entries with exactly this action */
  @query action?: AuditAction;

  /** Only entries about this target type, e.g. integration */
  @query targetType?: string;

  /** Only entries about the target with this id */
  @query targetId?: string;

  /** Only entries from this client address */
  @query ip?: string;

  /** Only entries made under this request id */
  @query requestId?: string;
}
