using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

@route("/workspaces/{workspaceId}/automations/{automationId}/enrollments")
@tag("Site Automations")
namespace SiteEnrollments {
  /** List an Automation's Enrollments, newest first */
  @get
  @requiresPermissions(#[])
  op list(
    @path workspaceId: WorkspaceId,
    @path automationId: EntityId,
    ...PageQuery,

    /** Only Enrollments in this status */
    @query status?: EnrollmentStatus,
  ): Page<EnrollmentResource> | Problem;

  /** Get one Enrollment of an Automation */
  @get
  @route("/{enrollmentId}")
  @requiresPermissions(#[])
  op get(
    @path workspaceId: WorkspaceId,
    @path automationId: EntityId,
    @path enrollmentId: EntityId,
  ): EnrollmentResource | Problem;
}

@route("/workspaces/{workspaceId}/automations/{automationId}/skipped-entries")
@tag("Site Automations")
namespace SiteSkippedEntries {
  /** List an automation's skipped entries, newest first: the entries its entry checks refused, each
   * with its reason */
  @get
  @requiresPermissions(#[])
  op list(
    @path workspaceId: WorkspaceId,
    @path automationId: EntityId,
    ...PageQuery,

    /** Only the skipped entries of this contact */
    @query contactId?: EntityId,
  ): Page<SkippedEntryResource> | Problem;
}
