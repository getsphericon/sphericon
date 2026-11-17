using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

@route("/automations/{automationId}/enrollments")
@tag("Enrollments")
namespace Enrollments {
  /** List an Automation's Enrollments, newest first. Needs the enrollments:read scope. */
  @get
  @requiresScopes(#[ApiTokenScope.enrollmentsRead])
  op list(
    @path automationId: EntityId,
    ...PageQuery,

    /** Only Enrollments in this status */
    @query status?: EnrollmentStatus,
  ): Page<EnrollmentResource> | Problem;

  /** Get one Enrollment of an Automation. Needs the enrollments:read scope. */
  @get
  @route("/{enrollmentId}")
  @requiresScopes(#[ApiTokenScope.enrollmentsRead])
  op get(@path automationId: EntityId, @path enrollmentId: EntityId):
    | EnrollmentResource
    | Problem;
}
