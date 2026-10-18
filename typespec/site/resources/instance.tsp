using TypeSpec.Http;

namespace Sphericon;

/** Why the environment, not the Owner, decides the Telemetry report */
enum SiteInstanceTelemetryLock {
  /** `SPHERICON_TELEMETRY` is set to true or false */
  environment: "environment",

  /** `DO_NOT_TRACK=1` switches the report off */
  doNotTrack: "do_not_track",
}

/** The Telemetry report setting as it applies right now (ADR 0032) */
model SiteInstanceTelemetry {
  /** Whether the Instance sends the Telemetry report */
  enabled: boolean;

  /** Whether the environment decides, which makes the in-app switch read-only */
  locked: boolean;

  /** What decides, set when locked */
  lockedBy?: SiteInstanceTelemetryLock;
}

/** What an Instance's Owner sees and decides about the Instance itself: it belongs to no
 * Workspace. Only the Owners of the oldest existing Workspace may read or change it. */
model SiteInstanceState {
  /** The Telemetry report setting */
  telemetry: SiteInstanceTelemetry;

  /** Whether the one-time notice about the Telemetry report was acknowledged */
  noticeAcknowledged: boolean;
}

/** Switch the Telemetry report on or off */
model SiteInstanceTelemetryInput {
  /** The Owner's choice */
  enabled: boolean;
}

@route("/instance")
@tag("Site Instance")
namespace SiteInstance {
  /** Read the Instance state. Refused to everyone but the Owners of the oldest Workspace. */
  @get
  op getState():
    | {
        @statusCode statusCode: 200;
        @body body: SiteInstanceState;
      }
    | ForbiddenProblem;

  /** Switch the Telemetry report on or off. Answers 409 while the environment decides. */
  @put
  @route("/telemetry")
  op setTelemetry(@body body: SiteInstanceTelemetryInput):
    | {
        @statusCode statusCode: 200;
        @body body: SiteInstanceState;
      }
    | ForbiddenProblem
    | ConflictProblem;

  /** Acknowledge the one-time Telemetry report notice. Idempotent. */
  @post
  @route("/notice/acknowledge")
  op acknowledgeNotice():
    | {
        @statusCode statusCode: 200;
        @body body: SiteInstanceState;
      }
    | ForbiddenProblem;

  /**
   * The Telemetry report exactly as the Instance would send it now (ADR 0032): the body the
   * published schema describes, built by the code that sends it. Works whether or not sending is
   * on. Refused to everyone but the Owners of the oldest Workspace.
   */
  @get
  @route("/telemetry/preview")
  op previewTelemetryReport():
    | {
        @statusCode statusCode: 200;
        @body body: Record<unknown>;
      }
    | ForbiddenProblem;
}
