using TypeSpec.Http;

namespace Sphericon;

/** Why the environment, not the Staff member, decides the Telemetry report */
enum StaffInstanceTelemetryLock {
  /** The Instance's Mode keeps the report off (the saas Mode) */
  environment: "environment",

  /** `DO_NOT_TRACK=1` switches the report off */
  doNotTrack: "do_not_track",
}

/** The Telemetry report setting as it applies right now (ADR 0032) */
model StaffInstanceTelemetry {
  /** Whether the Instance sends the Telemetry report */
  enabled: boolean;

  /** Whether the environment decides, which makes the switch in the console read-only */
  locked: boolean;

  /** What decides, set when locked */
  lockedBy?: StaffInstanceTelemetryLock;
}

/** The Instance's own settings and state, which belong to no Workspace (ADR 0063): what a Staff
 * member sees and decides about the Instance itself. */
model StaffInstanceState {
  /** The Telemetry report setting */
  telemetry: StaffInstanceTelemetry;

  /** Whether a Staff member acknowledged the one-time notice about the Telemetry report. No report
   * is sent before. */
  noticeAcknowledged: boolean;

  /** The newest stable release (a bare version such as `0.4.0`) when the Release check found
   * one newer than the running version; absent otherwise, and always absent for a build whose
   * version is not a release or in the saas Mode, which makes no Release check. */
  latestRelease?: string;

  /** When the Instance's license key expired, set only while it has lapsed: the Instance then
   * runs the open-source core, and nothing Enterprise is deleted (ADR 0035). */
  licenseExpiredAt?: Timestamp;

  /** The page that documents the schema of the Telemetry report */
  telemetrySchemaUrl: string;
}

/** Switch the Telemetry report on or off */
model StaffInstanceTelemetryInput {
  /** The Staff member's choice */
  enabled: boolean;
}

@route("/instance")
@tag("StaffMember Instance")
namespace StaffInstance {
  /** The Instance's own settings and state */
  @get
  op getState(): StaffInstanceState | UnauthorizedProblem;

  /** Switch the Telemetry report on or off. Answers 409 while the environment decides. */
  @put
  @route("/telemetry")
  op setTelemetry(@body body: StaffInstanceTelemetryInput):
    | StaffInstanceState
    | BadRequestProblem
    | UnauthorizedProblem
    | ConflictProblem;

  /** Acknowledge the one-time Telemetry report notice: the first report waits for it. The first
   * acknowledgement stands, and a repeat changes nothing. */
  @post
  @route("/notice/acknowledge")
  op acknowledgeNotice(): StaffInstanceState | UnauthorizedProblem;

  /**
   * The Telemetry report exactly as the Instance would send it now (ADR 0032): the body the
   * published schema describes, built by the code that sends it. Works whether or not sending is
   * on.
   */
  @get
  @route("/telemetry/preview")
  op previewTelemetryReport(): Record<unknown> | UnauthorizedProblem;
}
