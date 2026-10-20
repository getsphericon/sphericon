using TypeSpec.Http;

namespace Sphericon;

/** Who set a Workspace suspension (ADR 0026) */
enum StaffSuspensionActorKind {
  system,
  cli,
  staff,
}

/** The actor of a suspension. Unlike a customer, a Staff member sees the real Staff member id. */
model StaffSuspensionActor {
  kind: StaffSuspensionActorKind;

  /** The Staff member id; present only for the `staff` kind */
  id?: string | null;
}

/** A Workspace's suspension state (ADR 0007) */
model StaffSuspension {
  /** When outbound sending was suspended */
  at: Timestamp;

  /** Who suspended it */
  actor: StaffSuspensionActor;

  /** Why, as given by the actor */
  reason?: string | null;
}

/** A Workspace as the Staff console sees it: metadata only, never Contacts,
 * content or Events (ADR 0026). */
model StaffWorkspaceResource {
  /** Unique identifier */
  id: EntityId;

  slug: string;
  name: string;

  /** When the Workspace was created */
  createdAt: Timestamp;

  /** Present only while sending is suspended */
  suspension?: StaffSuspension | null;

  /** Exempt from the abuse detector (set by a Staff member) */
  exempt: boolean;

  /** The owners were warned and the Workspace has not left the warning tier since */
  warned: boolean;
}

/** Which metric of a Sending domain breached the abuse detector's warning limit */
enum StaffAbuseMetric {
  complaint,
  bounce,
}

/** A Sending domain in the warning tier (ADR 0026), by the detector's own computation */
model StaffAbuseWarning {
  domain: string;
  metric: StaffAbuseMetric;

  /** The rate, 0–1 */
  rate: float64;
}

/** A Workspace's standing with the abuse detector (ADR 0026) */
model StaffAbuse {
  /** Exempt from the detector: neither warned nor frozen automatically */
  exempt: boolean;

  /** When a Staff member last lifted a suspension; the detector counts only Events after it */
  clearedAt?: Timestamp | null;

  /** When the owners were told of the current warning; absent outside an episode */
  warningNotifiedAt?: Timestamp | null;

  /** When the owners were last told of a suspension */
  freezeNotifiedAt?: Timestamp | null;

  /** Present while the Workspace is in the warning tier, computed now */
  warning?: StaffAbuseWarning | null;
}

/** A rate as the (numerator, denominator, rate) triple (ADR 0011) */
model StaffRate {
  /** Events counted against the rate */
  numerator: int32;

  /** The population the rate is over */
  denominator: int32;

  /** numerator / denominator; null (undefined) when fewer sends than the volume floor
   * were made in the window, never reported as 0 */
  rate: float64 | null;
}

/** The Complaint and Bounce rates of one Sending domain over the trailing window */
model StaffDomainRates {
  /** Sending domain id */
  sendingDomainId: EntityId;

  /** The domain the rates are for */
  domain: string;

  /** Complaints over sent minus hard bounces */
  complaintRate: StaffRate;

  /** Hard bounces over sent */
  bounceRate: StaffRate;
}

/** A Workspace's deliverability over the trailing window: the rates of ADR 0011 per
 * Sending domain, and how much it sent. Counts only, never message content. */
model StaffDeliverability {
  /** Length of the trailing window, in hours */
  windowHours: int32;

  /** Fewest sends in the window (per Sending domain) for a rate to be defined */
  volumeFloor: int32;

  /** Messages sent in the window across all Sending domains */
  sendVolume: int32;

  /** One entry per Sending domain, oldest first */
  domains: StaffDomainRates[];
}

/** An active Staff hold on a Workspace, as the console shows it (read only) */
model StaffHold {
  /** Unique per Workspace, chosen by the setter */
  key: string;

  /** Machine reason code */
  reasonCode: string;

  /** The message the setter gave for the Owner */
  message: string;

  /** When the hold was first set */
  createdAt: Timestamp;

  /** When the hold lapses unless renewed */
  expiresAt: Timestamp;

  /** Who set it: the name of its Service credential */
  setBy: string;

  /** Where an Owner or Admin can resolve the hold, as the setter gave it; absent when none was given */
  actionUrl?: string | null;
}

/** A Workspace with its deliverability and active holds, as the detail page shows it */
model StaffWorkspaceDetailResource {
  ...StaffWorkspaceResource;

  /** Active Staff holds by key; a lapsed hold is not listed */
  holds: StaffHold[];

  deliverability: StaffDeliverability;
  abuse: StaffAbuse;
}

/** Which Workspaces the list surfaces for review */
enum StaffWorkspaceReview {
  /** The owners were warned and are still in the warning tier */
  warned,

  /** Suspended by the abuse detector (actor `system`) */
  systemFrozen,
}

/** Search filter of the Workspace list */
model StaffWorkspaceSearch {
  /** Only Workspaces in this review state */
  @query review?: StaffWorkspaceReview;

  /** Only Workspaces whose slug contains this text (case-insensitive) */
  @query slug?: string;
}

/** Why a Workspace is being suspended; the owner is told */
model StaffSuspendInput {
  /** The reason, required: the Workspace owner is told why sending stopped */
  @minLength(1)
  reason: string;
}

/** The outcome of an exempt or un-exempt: the Workspace as it now stands, and whether
 * anything changed */
model StaffExemptionChange {
  changed: boolean;
  workspace: StaffWorkspaceResource;
}

/** The outcome of a suspend or unsuspend: the Workspace as it now stands, and whether
 * anything changed (suspending a suspended Workspace and unsuspending an active one
 * change nothing). */
model StaffSuspensionChange {
  changed: boolean;
  workspace: StaffWorkspaceResource;
}

@tag("StaffMember Workspaces")
@route("/workspaces")
namespace StaffWorkspaces {
  /** List all Workspaces, newest first, optionally narrowed by a slug search */
  @get
  op list(...StaffWorkspaceSearch, ...PageQuery):
    | Page<StaffWorkspaceResource>
    | UnauthorizedProblem;

  /** One Workspace's metadata, suspension state, rates and send volume */
  @route("/{workspaceId}")
  @get
  op get(@path workspaceId: EntityId):
    | StaffWorkspaceDetailResource
    | UnauthorizedProblem
    | NotFoundProblem;

  /** Suspend a Workspace's outbound sending (ADR 0007) with a required reason, as the
   * signed-in Staff member. Idempotent: an already-suspended Workspace is left as it was. */
  @route("/{workspaceId}/suspend")
  @post
  op suspend(@path workspaceId: EntityId, @body body: StaffSuspendInput):
    | StaffSuspensionChange
    | UnauthorizedProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Lift a Workspace's suspension; held sends resume. Idempotent: a Workspace that is
   * not suspended is left as it was. */
  @route("/{workspaceId}/unsuspend")
  @post
  op unsuspend(@path workspaceId: EntityId):
    | StaffSuspensionChange
    | UnauthorizedProblem
    | NotFoundProblem;

  /** Exempt a Workspace from the abuse detector (ADR 0026): it is neither warned nor
   * frozen automatically, but stays listed with its rates and can still be suspended by
   * hand. Audited. Idempotent. */
  @route("/{workspaceId}/exempt")
  @post
  op exempt(@path workspaceId: EntityId):
    | StaffExemptionChange
    | UnauthorizedProblem
    | NotFoundProblem;

  /** Police a Workspace by the abuse detector again. Audited. Idempotent. */
  @route("/{workspaceId}/unexempt")
  @post
  op unexempt(@path workspaceId: EntityId):
    | StaffExemptionChange
    | UnauthorizedProblem
    | NotFoundProblem;
}
