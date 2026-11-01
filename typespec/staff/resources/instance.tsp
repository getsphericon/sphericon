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

  /** The Send cap every new Workspace starts with; absent on an Instance that has no Send caps */
  defaultSendCap?: StaffDefaultSendCap;
}

/** The default Send cap (ADR 0067): copied onto every Workspace created from now on. Changing it
 * leaves the Send caps of existing Workspaces as they are. */
model StaffDefaultSendCap {
  /** Messages per rolling 24 hours; null when new Workspaces get no Send cap */
  perDay: MaxPerDay | null;
}

/** The default Send cap a Staff member sets */
model StaffDefaultSendCapInput {
  /** Messages per rolling 24 hours */
  perDay: MaxPerDay;
}

/** The outcome of setting or clearing the default Send cap: the default as it now stands, and
 * whether anything changed (setting the same value, or clearing none, changes nothing) */
model StaffDefaultSendCapChange {
  changed: boolean;
  defaultSendCap: StaffDefaultSendCap;
}

/** What an entry of the Instance audit log says was changed, as `instance.<what>_change` */
enum InstanceAuditAction {
  /** Changed the default Send cap of new workspaces */
  defaultSendCapChange: "instance.default_send_cap_change",
}

/** One entry of the Instance audit log (ADR 0070): a Staff member changed a setting of the
 * Instance itself */
model StaffInstanceAuditEntryResource {
  /** Unique identifier */
  id: EntityId;

  /** When the change was made */
  occurredAt: Timestamp;

  /** The Staff member who made it */
  actor: StaffResource;

  /** What was changed */
  action: InstanceAuditAction;

  /** The old (`from`) and the new (`to`) value of each changed field; `from` is absent where there
   * was no value, `to` where it was cleared */
  diff: JsonObject;
}

/** A page of the Instance audit log, newest first */
model StaffInstanceAuditEntryList {
  items: StaffInstanceAuditEntryResource[];

  /** Pass as `cursor` for the next page; absent on the last page */
  nextCursor?: string | null;
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
  op previewTelemetryReport(): JsonObject | UnauthorizedProblem;

  /** Set the default Send cap (ADR 0067) as the signed-in Staff member: every Workspace created
   * from now on starts with it; existing Workspaces keep theirs. Recorded in the Instance audit
   * log. Idempotent. Not found on an Instance that has no Send caps. */
  @put
  @route("/default-send-cap")
  op setDefaultSendCap(@body body: StaffDefaultSendCapInput):
    | StaffDefaultSendCapChange
    | BadRequestProblem
    | UnauthorizedProblem
    | NotFoundProblem;

  /** Remove the default Send cap: Workspaces created from now on get no Send cap; existing
   * Workspaces keep theirs. Recorded in the Instance audit log. Idempotent. Not found on an
   * Instance that has no Send caps. */
  @delete
  @route("/default-send-cap")
  op clearDefaultSendCap():
    | StaffDefaultSendCapChange
    | UnauthorizedProblem
    | NotFoundProblem;

  /** The Instance audit log (ADR 0070), newest first: the changes Staff members made to the
   * Instance's own settings */
  @get
  @route("/audit-entries")
  op listAuditEntries(
    /** Cursor from a previous page */
    @query cursor?: string,

    /** Page size */
    @query
    @minValue(1)
    @maxValue(100)
    limit?: int32 = 25,
  ): StaffInstanceAuditEntryList | BadRequestProblem | UnauthorizedProblem;
}
