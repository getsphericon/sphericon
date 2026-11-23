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

@route("/automations/{automationId}/skipped-entries")
@tag("Automations")
namespace SkippedEntries {
  /** List an automation's skipped entries, newest first: the entries its entry checks refused, each
   * with its reason. Needs the automations:read scope. */
  @get
  @requiresScopes(#[ApiTokenScope.automationsRead])
  op list(
    @path automationId: EntityId,
    ...PageQuery,

    /** Only the skipped entries of this contact */
    @query contactId?: EntityId,
  ): Page<SkippedEntryResource> | Problem;
}
