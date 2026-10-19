using TypeSpec.Http;

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

/** The four Setup checklist steps, in the order the checklist shows them; none blocks another */
model SiteSetupSteps {
  /** At least one Integration exists */
  integration: SiteSetupStepStatus;

  /** At least one Sending domain is verified; pending while one exists unverified */
  sendingDomain: SiteSetupStepStatus;

  /** At least one Contact exists, from any source */
  contact: SiteSetupStepStatus;

  /** At least one Outbound message was sent, from any surface (a Test send is not one) */
  sentEmail: SiteSetupStepStatus;
}

/** The Workspace's Setup checklist: a point-in-time read model, never ticked by hand */
model SiteSetupProgress {
  steps: SiteSetupSteps;

  /** Every step is done */
  complete: boolean;

  /** An owner or admin dismissed the checklist for the whole Workspace */
  dismissed: boolean;
}

@route("/workspaces/{slug}/setup-checklist")
@tag("Site Setup Checklist")
namespace SiteSetupChecklist {
  /** The Workspace's Setup checklist */
  @get
  op get(@path slug: string): SiteSetupProgress | NotFoundProblem;
}
