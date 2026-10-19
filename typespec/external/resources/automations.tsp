using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

namespace Sphericon;

/** Automation lifecycle status */
enum AutomationStatus {
  draft,
  active,
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

/** An Automation: an event-triggered, ordered sequence of steps. Created
 * automations are always draft; activating is a separate, send-class operation. */
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

  /** Event action that enrolls a contact (e.g. "contact.created", "email.opened") */
  triggerEvent: string;

  /** The automation's ordered, linear steps */
  steps: AutomationStep[];

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Request body for creating an automation (always created as a draft) */
model CreateAutomationInput {
  name: string;
  triggerEvent: string;
  steps?: AutomationStep[];
}

/** Request body for updating an automation */
model UpdateAutomationInput {
  name?: string;
  triggerEvent?: string;
  steps?: AutomationStep[];
}

@tag("Automations")
interface Automations
  extends ExternalCrud<
      AutomationResource,
      CreateAutomationInput,
      UpdateAutomationInput
    > {
  /** Activate an automation (starts enrolling contacts). Needs the automations:activate scope. */
  @action("activate")
  @autoRoute
  activate(...ResourceParameters<AutomationResource>):
    | AutomationResource
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Deactivate an automation (stops new enrollments; in-flight ones finish). Needs the automations:activate scope. */
  @action("deactivate")
  @autoRoute
  deactivate(...ResourceParameters<AutomationResource>):
    | AutomationResource
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  /** Render an HTTP step's body for a Contact and validate it, calling nothing */
  @collectionAction(AutomationResource, "dry-run-http-step")
  @autoRoute
  dryRunHttpStep(
    ...ResourceCollectionParameters<AutomationResource>,
    @body body: DryRunHttpStepInput,
  ):
    | DryRunHttpStepResult
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;
}
