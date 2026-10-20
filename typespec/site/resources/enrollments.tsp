using TypeSpec.Http;

namespace Sphericon;

/** Where a Contact is in an Automation */
enum SiteEnrollmentStatus {
  active,
  completed,
  failed,
  exited,
}

/**
 * Why an Enrollment failed. A closed set: `invalid_body`, `timeout`, `blocked_address`,
 * `http_status` (see `status`) and `network`.
 */
enum SiteEnrollmentFailureKind {
  invalid_body: "invalid_body",
  timeout: "timeout",
  blocked_address: "blocked_address",
  http_status: "http_status",
  network: "network",
}

/** What an HTTP step recorded when it failed an Enrollment; absent on any other Enrollment. */
model SiteEnrollmentFailure {
  /** The closed error kind */
  kind: SiteEnrollmentFailureKind;

  /** HTTP status the receiver answered; present only for kind `http_status` */
  status?: int32;

  /** The first 1 KiB of the receiver's answer, when it sent one */
  excerpt?: string;
}

/** One Contact's progress through an Automation */
model SiteEnrollmentResource {
  /** Unique identifier */
  id: EntityId;

  /** The Automation the Contact was enrolled in */
  automationId: EntityId;

  /** The enrolled Contact */
  contactId: EntityId;

  /** Where the Enrollment is */
  status: SiteEnrollmentStatus;

  /** Index of the next step to run (0-based) */
  currentStep: int32;

  /** When a waiting Enrollment runs its next step */
  resumeAt?: Timestamp;

  /** Present on a failed Enrollment whose failure an HTTP step recorded */
  failure?: SiteEnrollmentFailure;

  /** When the Contact was enrolled */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

@route("/workspaces/{workspaceId}/automations/{automationId}/enrollments")
@tag("Site Automations")
namespace SiteEnrollments {
  /** List an Automation's Enrollments, newest first */
  @get
  op list(
    @path workspaceId: WorkspaceId,
    @path automationId: EntityId,
    ...PageQuery,

    /** Only Enrollments in this status */
    @query status?: SiteEnrollmentStatus,
  ): Page<SiteEnrollmentResource> | BadRequestProblem | NotFoundProblem;

  /** Get one Enrollment of an Automation */
  @get
  @route("/{enrollmentId}")
  op get(
    @path workspaceId: WorkspaceId,
    @path automationId: EntityId,
    @path enrollmentId: EntityId,
  ): SiteEnrollmentResource | BadRequestProblem | NotFoundProblem;
}
