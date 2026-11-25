using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** Automation resource used by the site UI */
@resource("automations")
@segment("automations")
@parentResource(SiteWorkspaceResource)
model SiteAutomationResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** Automation name */
  name: string;

  /** Lifecycle status */
  status: AutomationStatus;

  /** What it does now: the version new enrollments begin on; null until first published */
  live: AutomationVersion | null;

  /** Unpublished changes, the one version above live; null when there are none */
  draft: AutomationVersion | null;

  /** When the automation was archived; null while it is kept */
  archivedAt?: Timestamp | null;

  /** Orders credited to the automation and their net revenue (ADR 0064); set on the list and on
   * the single read, absent on the responses of a change */
  revenue?: MessageRevenue;

  /** The revision of the state this answer shows (ADR 0119): an update, Publish and Revert name it */
  revision: Revision;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** The state an automation is in, as the list shows it: draft until it is first published, then
 * active or inactive */
enum SiteAutomationState {
  draft,
  active,
  inactive,
}

/** What the automation list is ordered by */
enum SiteAutomationsSort {
  name,
  createdAt,
}

/** Filter and order of the automation list */
model SiteAutomationsListFilter {
  /** true lists the archived automations instead of the kept ones; the default hides them */
  @query archived?: boolean;

  /** Keeps the automations whose name contains this text, ignoring case */
  @query name?: string;

  /** Keeps the automations in this state */
  @query state?: SiteAutomationState;

  /** What the list is ordered by; the default is createdAt */
  @query sort?: SiteAutomationsSort;

  /** The direction of the order; the default is desc, the newest first, or asc when sorted by name */
  @query direction?: SortDirection;
}

/**
 * An automation is archived, never deleted (ADR 0100): it has no delete. An archived automation is
 * read-only until it is unarchived: an edit, publish, revert or deactivate of it is a 409.
 */
/** The Conditions of a Branch being edited, whose need of an unknown path is asked */
model BranchUnknownPathInput {
  /** The Conditions of the Branch's listed paths, in order */
  conditions: BranchConditionInput[];
}

/** Whether a Branch needs its unknown path (ADR 0124) */
model BranchUnknownPathResult {
  /**
   * True when one of its Conditions can be unknown for a Contact (it counts an open or a click,
   * ADR 0123): Publish then refuses the Branch without an unknown path, and with one when false
   */
  needed: boolean;
}

@tag("Site Automations")
interface SiteAutomations
  extends WorkspaceCrudArchivable<
      SiteAutomationResource,
      CreateAutomationInput,
      UpdateAutomationInput,
      #[],
      #[],
      SiteAutomationsListFilter
    > {
  /**
   * Publish an automation: the draft becomes live and the automation active, enrolling contacts;
   * with no draft, the live version is resumed. 422 with neither. 409 when any send step of the version
   * going live sends through an integration that cannot send (disabled, archived, of another
   * channel, or no default for the step's channel): a draft may name such an integration, only
   * publishing it is refused. 409 for an archived automation too, and 409 postal_address_required
   * when the version has an email send step and the Workspace has no postal address (ADR 0156).
   */
  @action("publish")
  @autoRoute
  @requiresPermissions(#[])
  publish(
    ...ResourceParameters<SiteAutomationResource>,
    @body body: RevisionInput,
  ): SiteAutomationResource | Problem;

  /** Drop the draft: the live version is again the latest. 422 when there is no draft or nothing is live, 409 when the automation is archived. */
  @action("revert")
  @autoRoute
  @requiresPermissions(#[])
  revert(
    ...ResourceParameters<SiteAutomationResource>,
    @body body: RevisionInput,
  ): SiteAutomationResource | Problem;

  /** Deactivate an automation (stops new enrollments). 409 when it is archived. */
  @action("deactivate")
  @autoRoute
  @requiresPermissions(#[])
  deactivate(...ResourceParameters<SiteAutomationResource>):
    | SiteAutomationResource
    | Problem;

  /**
   * Whether a Branch with these Conditions needs an unknown path, as Publish decides it: the editor
   * asks it of the version being edited
   */
  @collectionAction(SiteAutomationResource, "unknown-path")
  @autoRoute
  @requiresPermissions(#[])
  unknownPath(
    ...ResourceCollectionParameters<SiteAutomationResource>,
    @body body: BranchUnknownPathInput,
  ): BranchUnknownPathResult | Problem;

  /** Render an HTTP step's body for a Contact and validate it, calling nothing */
  @collectionAction(SiteAutomationResource, "dry-run-http-step")
  @autoRoute
  @requiresPermissions(#[])
  dryRunHttpStep(
    ...ResourceCollectionParameters<SiteAutomationResource>,
    @body body: DryRunHttpStepInput,
  ): DryRunHttpStepResult | Problem;
}

@route("/workspaces/{workspaceId}/automations/{automationId}/report")
@tag("Site Automations")
namespace SiteAutomationReports {
  /**
   * The opens and clicks of an automation's emails, with a count per clicked link, and its results
   * over a period. The engagement figures are the automation's whole history: opens and clicks come
   * from the `email.opened` and `email.clicked` Events, so they cover the Event retention window.
   * The results are read from its enrollments for the chosen period and the one before it.
   */
  @get
  @requiresPermissions(#[])
  op get(
    @path workspaceId: WorkspaceId,
    @path automationId: EntityId,

    /** The period the results are read over (defaults to 30d) */
    @query
    period?: AutomationReportPeriod = AutomationReportPeriod.`30d`,
  ): AutomationReport | Problem;
}

@@doc(
  SiteAutomations.archive,
  """
    Archive an automation: it leaves the list, enrolls nobody (an active one is deactivated) and
    keeps its enrollments. It can be unarchived, inactive. Refused with 409 while it has active
    enrollments or an active rule enrolls contacts in it (`usedBy` names them).
    """
);

@@doc(
  SiteAutomations.unarchive,
  "Unarchive an archived automation: it is back in the list, inactive, and only publish makes it enroll again"
);

@@doc(
  SiteAutomations.update,
  "Update an automation: its name, and its definition written to the draft. 409 when the automation is archived."
);
