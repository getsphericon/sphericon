using TypeSpec.Http;
using TypeSpec.OpenAPI;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

namespace Sphericon;

/** An Automation: an event-triggered, ordered sequence of steps. Edits write a draft beside the
 * live version, and only publishing (a send-class operation) changes what it does. Created
 * automations are always inactive, with the definition as their draft. */
@resource("automations")
@segment("automations")
model AutomationResource {
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

  /** The revision of the state this answer shows (ADR 0119): an update, Publish and Revert name it */
  revision: Revision;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Filter of the automation list */
model AutomationsListFilter {
  /** true lists the archived automations instead of the kept ones; the default hides them */
  @query archived?: boolean;
}

/**
 * An automation is archived, never deleted (ADR 0100): it has no delete. An archived automation is
 * read-only until it is unarchived: an edit, publish, revert or deactivate of it is a 409.
 */
@tag("Automations")
interface Automations
  extends ExternalCrudArchivable<
      AutomationResource,
      CreateAutomationInput,
      UpdateAutomationInput,
      ApiTokenScope.automationsRead,
      ApiTokenScope.automationsWrite,
      AutomationsListFilter
    > {
  /** Publish an automation: the draft becomes live and the automation active, enrolling contacts; with no draft, the live version is resumed. 422 with neither. 409 when any send step of the version going live sends through an integration that cannot send (disabled, archived, of another channel, or no default for the step's channel): a draft may name such an integration, only publishing it is refused. 409 for an archived automation too, and 409 postal_address_required when the version has an email send step and the Workspace has no postal address (`postalAddress` of `GET /workspace`, set only in the console, ADR 0156). Needs the automations:publish scope. */
  @action("publish")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsPublish])
  publish(
    ...ResourceParameters<AutomationResource>,
    @body body: RevisionInput,
  ): AutomationResource | Problem;

  /** Drop the draft: the live version is again the latest. 422 when there is no draft or nothing is live, 409 when the automation is archived. Needs the automations:write scope. */
  @action("revert")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsWrite])
  revert(...ResourceParameters<AutomationResource>, @body body: RevisionInput):
    | AutomationResource
    | Problem;

  /** Read the report: the engagement of the automation's emails over its whole history, and its
   * results over a period of Report days in the Workspace time zone (entered, completed,
   * goal_reached, exited and failed enrollments, their average duration and the revenue credited),
   * each with the period just before it. Needs the automations:read scope. */
  @action("report")
  @get
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsRead])
  report(
    ...ResourceParameters<AutomationResource>,

    /** The period the results are read over (defaults to 30d) */
    @query period?: AutomationReportPeriod = AutomationReportPeriod.`30d`,
  ): AutomationReport | Problem;

  /** Deactivate an automation (stops new enrollments; in-flight ones finish). 409 when it is archived. Needs the automations:write scope. */
  @action("deactivate")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsWrite])
  deactivate(...ResourceParameters<AutomationResource>):
    | AutomationResource
    | Problem;

  /** Render an HTTP step's body for a Contact and validate it, calling nothing */
  @collectionAction(AutomationResource, "dry-run-http-step")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsWrite, ApiTokenScope.contactsRead])
  dryRunHttpStep(
    ...ResourceCollectionParameters<AutomationResource>,
    @body body: DryRunHttpStepInput,
  ): DryRunHttpStepResult | Problem;
}

@@doc(
  Automations.archive,
  "Archive an automation: it leaves the list, enrolls nobody (an active one is deactivated) and keeps its enrollments. It can be unarchived, inactive. Refused with 409 while it has active enrollments or an active rule enrolls contacts in it (`usedBy` names them)."
);

@@doc(
  Automations.unarchive,
  "Unarchive an archived automation: it is back in the list, inactive, and only publish makes it enroll again"
);

@@doc(
  Automations.update,
  "Update an automation: its name, and its definition written to the draft. 409 when the automation is archived. Needs the automations:write scope."
);
