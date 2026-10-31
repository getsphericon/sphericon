using TypeSpec.Http;

namespace Sphericon;

/** The Instance's default Send cap (ADR 0067): the Send cap every Workspace created from now on
 * starts with. Changing it leaves existing Workspaces' Send caps as they are. */
model StaffSendCapDefault {
  /** Whether this Instance has Send caps; without, there is no default to set */
  sendCaps: boolean;

  /** The default in messages per rolling 24 hours; null while none is set, when new Workspaces
   * get no Send cap */
  sendCap: int32 | null;
}

/** The outcome of a change to the default Send cap: the default as it now stands, and whether
 * anything changed */
model StaffSendCapDefaultChange {
  changed: boolean;
  default: StaffSendCapDefault;
}

@route("/send-cap-default")
@tag("StaffMember Send cap default")
namespace StaffSendCapDefaults {
  /** The Instance's default Send cap, and whether the Instance has Send caps at all */
  @get
  op get(): StaffSendCapDefault | UnauthorizedProblem;

  /** Set the default Send cap: every Workspace created from now on starts with it; existing
   * Workspaces keep theirs. Only an Instance with Send caps has this operation (404 elsewhere).
   * Idempotent. */
  @put
  op set(@body body: StaffSendCapInput):
    | StaffSendCapDefaultChange
    | UnauthorizedProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Remove the default Send cap: Workspaces created from now on get none. 404 on an Instance
   * without Send caps. Idempotent. */
  @delete
  op clear(): StaffSendCapDefaultChange | UnauthorizedProblem | NotFoundProblem;
}
