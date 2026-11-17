import "../../common/audit-log.tsp";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

@route("/audit-entries")
@tag("Audit")
namespace AuditEntries {
  /** List the Workspace's Audit log, newest first (Enterprise; needs the `audit:read` scope) */
  @get
  @requiresScopes(#[ApiTokenScope.auditRead])
  op list(...AuditFilter, ...CursorQuery): AuditEntryList | Problem;
}
