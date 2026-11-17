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
