using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

namespace Sphericon;

/** Automation lifecycle status */
enum SiteAutomationStatus {
  draft,
  active,
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
  /** The Liquid JSON body to render */
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

  /** Email body as MJML (email steps), or the Liquid JSON request body (HTTP steps: Contact bindings only, strings JSON-escaped, up to 64 KiB, must render to valid JSON) */
  body?: string;

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

  /** Event action that enrolls a contact (e.g. "contact.created", "email.opened") */
  triggerEvent: string;

  /** The automation's ordered, linear steps */
  steps: SiteAutomationStep[];

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Site request body for creating an automation */
model SiteCreateAutomationInput {
  name: string;
  triggerEvent: string;
  steps?: SiteAutomationStep[];
}

/** Site request body for updating an automation */
model SiteUpdateAutomationInput {
  name?: string;
  triggerEvent?: string;
  steps?: SiteAutomationStep[];
}

@tag("Site Automations")
interface SiteAutomations
  extends WorkspaceCrud<
      SiteAutomationResource,
      SiteCreateAutomationInput,
      SiteUpdateAutomationInput
    > {
  /** Activate an automation (starts enrolling contacts) */
  @action("activate")
  @autoRoute
  activate(...ResourceParameters<SiteAutomationResource>):
    | SiteAutomationResource
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Deactivate an automation (stops new enrollments) */
  @action("deactivate")
  @autoRoute
  deactivate(...ResourceParameters<SiteAutomationResource>):
    | SiteAutomationResource
    | BadRequestProblem
    | NotFoundProblem;

  /** Render an HTTP step's body for a Contact and validate it, calling nothing */
  @collectionAction(SiteAutomationResource, "dry-run-http-step")
  @autoRoute
  dryRunHttpStep(
    ...ResourceCollectionParameters<SiteAutomationResource>,
    @body body: SiteDryRunHttpStepInput,
  ):
    | SiteDryRunHttpStepResult
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;
}
