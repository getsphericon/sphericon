using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** Where one Setup checklist step stands, read off the Workspace's own data */
enum SiteSetupStepStatus {
  /** Nothing yet */
  todo,

  /** In progress: only the Sending domain step is ever pending (a domain exists, DNS has not verified it) */
  pending,

  /** Done */
  done,
}

/** The Setup checklist steps, in the order the checklist shows them; none blocks another */
model SiteSetupSteps {
  /** At least one real Integration exists; a Test inbox delivers nothing outside, so it does not count */
  integration: SiteSetupStepStatus;

  /** At least one Sending domain is verified; pending while one exists unverified */
  sendingDomain: SiteSetupStepStatus;

  /** At least one Contact exists, from any source */
  contact: SiteSetupStepStatus;

  /** At least one Outbound message was sent, from any surface (a Test send is not one) */
  sentEmail: SiteSetupStepStatus;
}

/** The Workspace's Setup checklist: its steps are read now, never ticked by hand, and the read
 * that first finds every step done stamps the Workspace, which keeps it complete for good */
model SiteSetupProgress {
  steps: SiteSetupSteps;

  /** When every step was first found done; null until then. A step that regresses later does not undo it */
  completedAt?: Timestamp | null;
}

@route("/workspaces/{workspaceId}/setup-checklist")
@tag("Site Setup Checklist")
namespace SiteSetupChecklist {
  /** The Workspace's Setup checklist */
  @get
  @requiresPermissions(#[])
  op get(@path workspaceId: WorkspaceId):
    | SiteSetupProgress
    | BadRequestProblem
    | NotFoundProblem;
}
