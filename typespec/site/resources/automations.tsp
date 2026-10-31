using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** Automation lifecycle status: active enrolls Contacts, inactive enrolls nobody */
enum SiteAutomationStatus {
  active,
  inactive,
}

/** Kind of automation step */
enum SiteAutomationStepType {
  email,
  wait,
  apply_tag,
  remove_tag,
  http,
}

/** One request header of an HTTP step. The value is a secret: it is never returned. */
model SiteAutomationHttpHeader {
  /** Header name. Content-Type, Content-Length, Host and the webhook-* headers are set by sphericon. */
  name: string;

  /** Header value. Omit it when editing to keep the stored value of the header with this name. */
  value?: string;
}

/** Closed error kind of an HTTP step's body */
enum SiteAutomationHttpErrorKind {
  invalid_body,
}

/** Request body for rendering an HTTP step's body for a Contact */
model SiteDryRunHttpStepInput {
  /** The Liquid JSON body to render. No Enrollment stands behind a dry run, so `event` is empty */
  body: string;

  /** The Contact whose values fill the template */
  contactId: EntityId;
}

/** The body rendered for a Contact, or the kind of error that stopped it. Nothing is called. */
model SiteDryRunHttpStepResult {
  /** The rendered, valid JSON body. Absent when errorKind is set. */
  body?: string;

  /** Why the body could not be rendered */
  errorKind?: SiteAutomationHttpErrorKind;

  /** Human-readable detail of the error */
  detail?: string;
}

/** One node in an automation's ordered, linear sequence. A flat shape: email
 * steps carry subject/body, wait steps carry seconds. */
model SiteAutomationStep {
  /** Step kind */
  type: SiteAutomationStepType;

  /** Email subject (email steps) */
  subject?: string;

  /** Email body as HTML (email steps), or the Liquid JSON request body (HTTP steps: strings JSON-escaped, up to 64 KiB, must render to valid JSON, also with every tag empty). Both bind the Contact's fields plus `event.action` and `event.properties`: the Event the Enrollment began on (the one a Rule fired on, when a Rule enrolled), copied when it began; empty when no Event began it */
  body?: string;

  /** The editor's JSON document an email step's body was built from, an opaque object checked by the
   * server. Write it instead of `body` (both is a 422): the server renders the body from it in the
   * same save, and a document that cannot be rendered refuses the whole save, naming the step.
   * Writing only a body clears the step's document. */
  doc?: Record<unknown>;

  /** Version of the editor that rendered the document (email steps, read only) */
  editorVersion?: string;

  /** Delay before the next step, in seconds (wait steps) */
  seconds?: int32;

  /** Tag name to apply or remove (apply_tag and remove_tag steps) */
  tag?: string;

  /** Step id (HTTP steps). Assigned on save; send it back on edit so the step keeps its secrets. */
  id?: string;

  /** Hook URL to POST to (HTTP steps). A secret: never returned. Omit it when editing to keep the stored URL. Up to 2 KiB, http or https. */
  url?: string;

  /** Up to 5 request headers (HTTP steps). On read, only the names are returned. */
  headers?: SiteAutomationHttpHeader[];

  /** Signing secret (HTTP steps): when set, the request is signed with Standard Webhooks. Never returned. Omit it when editing to keep the stored secret. */
  secret?: string;

  /** Remove the signing secret on save (HTTP steps) */
  clearSecret?: boolean;

  /** Whether a signing secret is stored (HTTP steps, read only) */
  hasSecret?: boolean;
}

/** One version of an automation's definition: what publishing makes it do (ADR 0065) */
model SiteAutomationVersion {
  /** 1 for the first version, plus one per version after it */
  number: int32;

  /** Event action that enrolls a contact (e.g. "contact.created", "email.opened") */
  triggerEvent: string;

  /** Event actions that end an active enrollment early, as exited (e.g. "plan_downgraded"). Only
   * an Event that occurred once the enrollment began counts; machine opens and clicks never do. */
  exitEvents: string[];

  /** Event action that ends an active enrollment as goal_reached (e.g. "payment_succeeded"); null
   * when there is none */
  goalEvent: string | null;

  /** Sending integration of the email steps; null means the workspace default */
  integrationId?: EntityId | null;

  /** The ordered, linear steps */
  steps: SiteAutomationStep[];
}

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
  status: SiteAutomationStatus;

  /** What it does now: the version new enrollments begin on; null until first published */
  live: SiteAutomationVersion | null;

  /** Unpublished changes, the one version above live; null when there are none */
  draft: SiteAutomationVersion | null;

  /** When the automation was archived; null while it is kept */
  archivedAt?: Timestamp | null;

  /** Orders credited to the automation and their net revenue (ADR 0064); set on the list and on
   * the single read, absent on the responses of a change */
  revenue?: SiteMessageRevenue;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Site request body for creating an automation */
model SiteCreateAutomationInput {
  name: string;
  triggerEvent: string;

  /** Event actions that end an active enrollment early; none of them may be the trigger. Each
   * follows the trigger's rule: any customer action, and of the email events only opened, clicked
   * and unsubscribed. */
  exitEvents?: string[];

  /** Event action that ends an active enrollment as goal_reached; it may be neither the trigger nor
   * an exit event. Null or omitted is none. */
  goalEvent?: string | null;

  steps?: SiteAutomationStep[];

  /** Sending integration of the email steps; null/omitted means the workspace default */
  integrationId?: EntityId | null;
}

/** Site request body for updating an automation. The name changes at once; the trigger, steps
 * and integration write the draft (ADR 0065): omitted fields keep the draft's, else the live
 * version's, and publishing makes the draft live. */
model SiteUpdateAutomationInput {
  name?: string;
  triggerEvent?: string;

  /** Event actions that end an active enrollment early; replaces the list, omitted keeps it */
  exitEvents?: string[];

  /** Event action that ends an active enrollment as goal_reached; null removes it, omitted keeps it */
  goalEvent?: string | null;

  steps?: SiteAutomationStep[];

  /** Sending integration of the email steps; null means the workspace default, omitted keeps it */
  integrationId?: EntityId | null;
}

/** Filter of the automation list */
model SiteAutomationsListFilter {
  /** true lists the archived automations instead of the kept ones; the default hides them */
  @query archived?: boolean;
}

/**
 * Delete removes an automation that has no enrollments and has sent nothing for good; one with
 * either is history and is archived instead (ADR 0049), so a delete of it is a 409 whose `usedBy`
 * carries their counts.
 */
@tag("Site Automations")
interface SiteAutomations
  extends WorkspaceCrud<
      SiteAutomationResource,
      SiteCreateAutomationInput,
      SiteUpdateAutomationInput,
      never,
      InUseProblem,
      SiteAutomationsListFilter
    > {
  /**
   * Archive an automation: it leaves the list, enrolls nobody (an active one is deactivated) and
   * keeps its enrollments. It can be restored, inactive. Refused with 409 while it has active
   * enrollments (`usedBy` carries their count).
   */
  @action("archive")
  @autoRoute
  @extension("x-required-permissions", #[])
  archive(...ResourceParameters<SiteAutomationResource>):
    | SiteAutomationResource
    | BadRequestProblem
    | NotFoundProblem
    | InUseProblem;

  /** Restore an archived automation to the list, inactive: only publish makes it enroll again */
  @action("unarchive")
  @autoRoute
  @extension("x-required-permissions", #[])
  unarchive(...ResourceParameters<SiteAutomationResource>):
    | SiteAutomationResource
    | BadRequestProblem
    | NotFoundProblem;

  /**
   * Publish an automation: the draft becomes live and the automation active, enrolling contacts;
   * with no draft, the live version is resumed. 422 with neither.
   */
  @action("publish")
  @autoRoute
  @extension("x-required-permissions", #[])
  publish(...ResourceParameters<SiteAutomationResource>):
    | SiteAutomationResource
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Drop the draft: the live version is again the latest. 422 when there is no draft or nothing is live. */
  @action("revert")
  @autoRoute
  @extension("x-required-permissions", #[])
  revert(...ResourceParameters<SiteAutomationResource>):
    | SiteAutomationResource
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Deactivate an automation (stops new enrollments) */
  @action("deactivate")
  @autoRoute
  @extension("x-required-permissions", #[])
  deactivate(...ResourceParameters<SiteAutomationResource>):
    | SiteAutomationResource
    | BadRequestProblem
    | NotFoundProblem;

  /** Render an HTTP step's body for a Contact and validate it, calling nothing */
  @collectionAction(SiteAutomationResource, "dry-run-http-step")
  @autoRoute
  @extension("x-required-permissions", #[])
  dryRunHttpStep(
    ...ResourceCollectionParameters<SiteAutomationResource>,
    @body body: SiteDryRunHttpStepInput,
  ):
    | SiteDryRunHttpStepResult
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;
}

/** An automation's engagement figures */
model SiteAutomationReport {
  /** Opens recorded on the automation's emails, every open counted; bounded by Event retention */
  opens: int32;

  /** Clicks recorded on the automation's emails, every click counted; bounded by Event retention */
  clicks: int32;

  /** Messages the provider reported delivered, read from the delivery result stamped on each message; not bounded by Event retention */
  delivered: int32;

  /** Messages the provider reported bounced, read from the delivery result stamped on each message; not bounded by Event retention */
  bounced: int32;

  /** Messages a recipient complained about, read from the delivery result stamped on each message; not bounded by Event retention */
  complained: int32;

  /** Messages that went through a Test inbox: captured, not delivered; they count as delivered too */
  captured: int32;

  /** Orders credited to the automation and their net revenue (ADR 0064) */
  revenue: SiteMessageRevenue;

  /** One entry per clicked link, most clicked first */
  links: SiteLinkClicks[];
}

@route("/workspaces/{workspaceId}/automations/{automationId}/report")
@tag("Site Automations")
namespace SiteAutomationReports {
  /**
   * The opens and clicks of an automation's emails, with a count per clicked link. They come from
   * the `email.opened` and `email.clicked` Events, so they cover the Event retention window.
   */
  @get
  @extension("x-required-permissions", #[])
  op get(@path workspaceId: WorkspaceId, @path automationId: EntityId):
    | SiteAutomationReport
    | BadRequestProblem
    | NotFoundProblem;
}

@@extension(SiteAutomations.create, "x-required-permissions", #[]);
@@extension(SiteAutomations.delete, "x-required-permissions", #[]);
@@extension(SiteAutomations.get, "x-required-permissions", #[]);
@@extension(SiteAutomations.list, "x-required-permissions", #[]);
@@extension(SiteAutomations.update, "x-required-permissions", #[]);
