using TypeSpec.Http;
using TypeSpec.OpenAPI;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

namespace Sphericon;

/** Automation lifecycle status: active enrolls Contacts, inactive enrolls nobody */
enum AutomationStatus {
  active,
  inactive,
}

/** Kind of automation step */
enum AutomationStepType {
  email,
  wait,
  apply_tag,
  remove_tag,
  http,
}

/** One request header of an HTTP step. The value is a secret: it is never returned. */
model AutomationHttpHeader {
  /** Header name. Content-Type, Content-Length, Host and the webhook-* headers are set by sphericon. */
  name: string;

  /** Header value. Omit it when editing to keep the stored value of the header with this name. */
  value?: string;
}

/** Closed error kind of an HTTP step's body */
enum AutomationHttpErrorKind {
  invalid_body,
}

/** Request body for rendering an HTTP step's body for a Contact */
model DryRunHttpStepInput {
  /** The Liquid JSON body to render. No Enrollment stands behind a dry run, so `event` is empty */
  body: string;

  /** The Contact whose values fill the template */
  contactId: EntityId;
}

/** The body rendered for a Contact, or the kind of error that stopped it. Nothing is called. */
model DryRunHttpStepResult {
  /** The rendered, valid JSON body. Absent when errorKind is set. */
  body?: string;

  /** Why the body could not be rendered */
  errorKind?: AutomationHttpErrorKind;

  /** Human-readable detail of the error */
  detail?: string;
}

/** One node in an automation's ordered, linear sequence. A flat shape: email
 * steps carry subject/body, wait steps carry seconds, tag steps carry the tag name. */
model AutomationStep {
  /** Step kind */
  type: AutomationStepType;

  /** Email subject (email steps) */
  subject?: string;

  /** Email body as HTML (email steps), or the Liquid JSON request body (HTTP steps: strings JSON-escaped, up to 64 KiB, must render to valid JSON, also with every tag empty). Both bind the Contact's fields plus `event.action` and `event.properties`: the Event the Enrollment began on (the one a Rule fired on, when a Rule enrolled), copied when it began; empty when no Event began it */
  body?: string;

  /** The editor's JSON document an email step's body was built from, an opaque object checked by the
   * server against the schema `email_schema` returns. Write it instead of `body` (both is a 422): the
   * server renders the body from it in the same save, and a document that cannot be rendered refuses
   * the whole save, naming the step. Writing only a body clears the step's document. */
  doc?: EditorDocument;

  /** Version of the editor that rendered the document (email steps, read only) */
  editorVersion?: string;

  /** Delay before the next step, in seconds (wait steps) */
  seconds?: int32;

  /** Tag name to apply to or remove from the enrolled contact (apply_tag and remove_tag steps) */
  tag?: string;

  /** Step id (HTTP steps). Assigned on save; send it back on edit so the step keeps its secrets. */
  id?: string;

  /** Hook URL to POST to (HTTP steps). A secret: never returned. Omit it when editing to keep the stored URL. Up to 2 KiB, http or https. */
  url?: string;

  /** Up to 5 request headers (HTTP steps). On read, only the names are returned. */
  headers?: AutomationHttpHeader[];

  /** Signing secret (HTTP steps): when set, the request is signed with Standard Webhooks. Never returned. Omit it when editing to keep the stored secret. */
  secret?: string;

  /** Remove the signing secret on save (HTTP steps) */
  clearSecret?: boolean;

  /** Whether a signing secret is stored (HTTP steps, read only) */
  hasSecret?: boolean;
}

/** One version of an automation's definition: what publishing makes it do (ADR 0065) */
model AutomationVersion {
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
  steps: AutomationStep[];
}

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

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Request body for creating an automation (always created inactive) */
model CreateAutomationInput {
  name: string;
  triggerEvent: string;

  /** Event actions that end an active enrollment early; none of them may be the trigger. Each
   * follows the trigger's rule: any customer action, and of the email events only opened, clicked
   * and unsubscribed. */
  exitEvents?: string[];

  /** Event action that ends an active enrollment as goal_reached; it may be neither the trigger nor
   * an exit event. Null or omitted is none. */
  goalEvent?: string | null;

  steps?: AutomationStep[];

  /** Sending integration of the email steps; null/omitted means the workspace default */
  integrationId?: EntityId | null;
}

/** Request body for updating an automation. The name changes at once; the trigger, steps and
 * integration write the draft (ADR 0065): omitted fields keep the draft's, else the live
 * version's, and only publishing makes the draft live. */
model UpdateAutomationInput {
  name?: string;
  triggerEvent?: string;

  /** Event actions that end an active enrollment early; replaces the list, omitted keeps it */
  exitEvents?: string[];

  /** Event action that ends an active enrollment as goal_reached; null removes it, omitted keeps it */
  goalEvent?: string | null;

  steps?: AutomationStep[];

  /** Sending integration of the email steps; null means the workspace default, omitted keeps it */
  integrationId?: EntityId | null;
}

/** Filter of the automation list */
model AutomationsListFilter {
  /** true lists the archived automations instead of the kept ones; the default hides them */
  @query archived?: boolean;
}

/** Orders credited to a broadcast or an automation and their net revenue (ADR 0064) */
model MessageRevenue {
  /** The Reporting currency, null until an owner or admin chooses it; every figure is zero until then */
  reportingCurrency: CurrencyCode | null;

  /** Orders completed that are credited to it, in the Reporting currency */
  orders: int32;

  /** Net revenue of those Orders, in the minor unit of the Reporting currency (cents for USD) */
  amount: int64;
}

/** How often one link of an automation's emails was clicked */
model LinkClicks {
  /** The link's destination URL */
  url: string;

  /** Clicks recorded on it, every click counted (not unique per contact); bounded by Event retention */
  clicks: int32;
}

/** The length of the period an automation's results are read over, in Report days (ADR 0057), today included */
enum AutomationReportPeriod {
  `7d`,
  `14d`,
  `30d`,
  `90d`,
}

/** How many enrollments began, and how many ended with each outcome */
model AutomationResultCounts {
  /** Enrollments that began: their triggering Event occurred in it */
  entered: int32;

  /** Enrollments that walked every step and ended in it */
  completed: int32;

  /** Enrollments that ended in it because their goal event was recorded */
  goalReached: int32;

  /** Enrollments that left early in it: an unsubscribe, a suppression, a hard bounce or an exit event */
  exited: int32;

  /** Enrollments that ended in it on a step that failed */
  failed: int32;
}

/**
 * One period of an automation's results. Each count is what happened in the period: an
 * enrollment counts as entered on the Report day it began and under its outcome on the Report day
 * it ended, whenever it began, so an enrollment still running counts as entered only.
 */
model AutomationPeriodResults {
  ...AutomationResultCounts;

  /** The period's first Report day, `YYYY-MM-DD` in the Workspace time zone */
  from: string;

  /** The period's last Report day, included */
  to: string;

  /** Mean time in whole seconds from the beginning to the end of the enrollments that ended in
   * the period, whatever their outcome; null when none ended */
  averageDurationSeconds: int64 | null;

  /** The orders credited to the automation that were completed in the period, and the net change
   * of its revenue in the period: every Movement of a credited Order counts on its own day (ADR 0064) */
  revenue: MessageRevenue;
}

/** One Report day of an automation's results */
model AutomationResultsPoint {
  ...AutomationResultCounts;

  /** The Report day, `YYYY-MM-DD` in the Workspace time zone */
  date: string;
}

/** An automation's results over a period of Report days, read from its enrollments, with the
 * period just before it to compare with */
model AutomationResults {
  /** The period the results are read over */
  period: AutomationReportPeriod;

  /** The period ending today */
  current: AutomationPeriodResults;

  /** As many Report days just before it */
  previous: AutomationPeriodResults;

  /** The current period by Report day, oldest first, a day with nothing at zero */
  series: AutomationResultsPoint[];
}

/** An automation's engagement figures over its whole history and its results over a period */
model AutomationReport {
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

  /** Orders credited to the automation and their net revenue, over its whole history (ADR 0064) */
  revenue: MessageRevenue;

  /** One entry per clicked link, most clicked first */
  links: LinkClicks[];

  /** What the automation's enrollments did over the chosen period and the period before it */
  results: AutomationResults;
}

/**
 * Delete removes an automation that has no enrollments and has sent nothing for good; one with
 * either is history and is archived instead (ADR 0049), so a delete of it is a 409 whose `usedBy`
 * carries their counts.
 */
@tag("Automations")
interface Automations
  extends ExternalCrud<
      AutomationResource,
      CreateAutomationInput,
      UpdateAutomationInput,
      ApiTokenScope.automationsRead,
      ApiTokenScope.automationsWrite,
      ApiTokenScope.automationsWrite,
      never,
      InUseProblem,
      AutomationsListFilter
    > {
  /** Archive an automation: it leaves the list, enrolls nobody (an active one is deactivated) and keeps its enrollments. It can be restored, inactive. Refused with 409 while it has active enrollments (`usedBy` carries their count). */
  @action("archive")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsWrite])
  archive(...ResourceParameters<AutomationResource>):
    | AutomationResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | InUseProblem;

  /** Restore an archived automation to the list, inactive: only publish makes it enroll again */
  @action("unarchive")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsWrite])
  unarchive(...ResourceParameters<AutomationResource>):
    | AutomationResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  /** Publish an automation: the draft becomes live and the automation active, enrolling contacts; with no draft, the live version is resumed. 422 with neither. 409 when the version going live has an email step and the integration it sends through cannot send (disabled or archived, or no default): a draft may name such an integration, only publishing it is refused. Needs the automations:publish scope. */
  @action("publish")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsPublish])
  publish(...ResourceParameters<AutomationResource>):
    | AutomationResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | ConflictProblem
    | UnprocessableEntityProblem;

  /** Drop the draft: the live version is again the latest. 422 when there is no draft or nothing is live. Needs the automations:write scope. */
  @action("revert")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsWrite])
  revert(...ResourceParameters<AutomationResource>):
    | AutomationResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

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
  ):
    | AutomationReport
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  /** Deactivate an automation (stops new enrollments; in-flight ones finish). Needs the automations:write scope. */
  @action("deactivate")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsWrite])
  deactivate(...ResourceParameters<AutomationResource>):
    | AutomationResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  /** Render an HTTP step's body for a Contact and validate it, calling nothing */
  @collectionAction(AutomationResource, "dry-run-http-step")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsWrite, ApiTokenScope.contactsRead])
  dryRunHttpStep(
    ...ResourceCollectionParameters<AutomationResource>,
    @body body: DryRunHttpStepInput,
  ):
    | DryRunHttpStepResult
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;
}
