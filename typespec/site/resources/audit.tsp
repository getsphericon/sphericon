import "../../common/audit-log.tsp";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** The values that occur in a Workspace's Audit log, offered as the choices of its filter */
model SiteAuditFacets {
  /** Every actor with an entry in the log, by kind, then name, then id; one id-less staff member stands for the platform staff, and each actor carries its latest name snapshot */
  actors: AuditActor[];

  /** Every action with an entry in the log, sorted */
  actions: AuditAction[];

  /** Every target type with an entry in the log, sorted */
  targetTypes: string[];
}

/** A number of days an Audit log is kept: 1 to 3650 */
@minValue(1)
@maxValue(3650)
scalar RetentionDays extends int32;

/** How long the Audit log is kept; the Enterprise advanced-retention control */
model SiteAuditRetention {
  /** Entries older than this many days are pruned; null keeps them forever */
  retentionDays: RetentionDays | null;

  /** The Workspace's revision (ADR 0119): what a read answers and a change names, the one it was read with */
  revision: Revision;
}

model SiteAuditRetentionInput is SiteAuditRetention;

@route("/workspaces/{workspaceId}/audit-entries")
@tag("Site Audit")
namespace SiteAudit {
  /** List the workspace's Audit log, newest first (Enterprise; owner and admin only) */
  @get
  @requiresPermissions(#[Permission.auditRead])
  op list(@path workspaceId: WorkspaceId, ...AuditFilter, ...CursorQuery):
    | AuditEntryList
    | Problem;

  /** The actors, actions and target types that occur in the Audit log, for its filter (Enterprise; owner and admin only) */
  @get
  @route("/facets")
  @requiresPermissions(#[Permission.auditRead])
  op facets(@path workspaceId: WorkspaceId): SiteAuditFacets | Problem;

  /** Export the Audit log as CSV, newest first, narrowed by the same filter as the list (Enterprise; owner and admin only) */
  @get
  @route("/export")
  @requiresPermissions(#[Permission.auditRead])
  op export(@path workspaceId: WorkspaceId, ...AuditFilter):
    | {
        @header contentType: "text/csv";
        @header contentDisposition: string;
        @body body: string;
      }
    | Problem;

  /** Read the Audit log retention window (Enterprise; owner and admin only) */
  @get
  @route("/retention")
  @requiresPermissions(#[Permission.auditConfigure])
  op getRetention(@path workspaceId: WorkspaceId): SiteAuditRetention | Problem;

  /** Set or clear the Audit log retention window; recorded as an Audit entry (Enterprise; owner and admin only) */
  @put
  @route("/retention")
  @requiresPermissions(#[Permission.auditConfigure])
  op setRetention(
    @path workspaceId: WorkspaceId,
    @body body: SiteAuditRetentionInput,
  ): SiteAuditRetention | Problem;
}
