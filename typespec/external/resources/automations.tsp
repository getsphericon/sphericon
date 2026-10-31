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
  /** The Liquid JSON body to render */
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

  /** Email body as HTML (email steps), or the Liquid JSON request body (HTTP steps: Contact bindings only, strings JSON-escaped, up to 64 KiB, must render to valid JSON) */
  body?: string;

  /** The editor's JSON document an email step's body was built from, an opaque object checked by the
   * server against the schema `email_schema` returns. Write it instead of `body` (both is a 422): the
   * server renders the body from it in the same save, and a document that cannot be rendered refuses
   * the whole save, naming the step. Writing only a body clears the step's document. */
  doc?: Record<unknown>;

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
  @extension("x-required-scopes", #[ApiTokenScope.automationsWrite])
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
  @extension("x-required-scopes", #[ApiTokenScope.automationsWrite])
  unarchive(...ResourceParameters<AutomationResource>):
    | AutomationResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  /** Publish an automation: the draft becomes live and the automation active, enrolling contacts; with no draft, the live version is resumed. 422 with neither. Needs the automations:publish scope. */
  @action("publish")
  @autoRoute
  @extension("x-required-scopes", #[ApiTokenScope.automationsPublish])
  publish(...ResourceParameters<AutomationResource>):
    | AutomationResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Drop the draft: the live version is again the latest. 422 when there is no draft or nothing is live. Needs the automations:write scope. */
  @action("revert")
  @autoRoute
  @extension("x-required-scopes", #[ApiTokenScope.automationsWrite])
  revert(...ResourceParameters<AutomationResource>):
    | AutomationResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Deactivate an automation (stops new enrollments; in-flight ones finish). Needs the automations:write scope. */
  @action("deactivate")
  @autoRoute
  @extension("x-required-scopes", #[ApiTokenScope.automationsWrite])
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
  @extension(
    "x-required-scopes",
    #[ApiTokenScope.automationsWrite, ApiTokenScope.contactsRead]
  )
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
