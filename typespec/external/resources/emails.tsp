using TypeSpec.Http;
using TypeSpec.Rest;

namespace Sphericon;

/** Outcome of a transactional send */
enum TransactionalSendStatus {
  /** Accepted by the email provider */
  sent,

  /** Skipped: the destination is on the workspace suppression list */
  suppressed,
}

/** Send a single transactional email by rendering a referenced template. */
model SendTransactionalEmailInput {
  /** The Template to render at send time (bound by reference, not copied) */
  templateId: EntityId;

  /** The destination email address */
  destination: EmailAddress;

  /** Per-call variables merged into the template's subject and body (Liquid `{{ }}`) */
  variables?: Record<unknown>;

  /**
   * Track opens and clicks of this message (default false). The email then carries an open
   * pixel and its links are rewritten; it stays transactional: no unsubscribe link, the same
   * eligibility and Suppression. Events attach to the Contact the address resolves to, else
   * are kept by address.
   */
  track?: boolean = false;
}

/** Result of a transactional send */
model SendTransactionalEmailResponse {
  /** The id of the durable send record (the send-history trace) */
  id: EntityId;

  /** Whether the message was sent or skipped as suppressed */
  status: TransactionalSendStatus;

  /** The normalized destination the result applies to */
  destination: string;
}

/** The message to lint */
model LintEmailInput {
  /** Subject line, with Liquid merge tags */
  subject?: string;

  /** HTML body with Liquid merge tags. Send this or `doc`, not both. */
  body?: string;

  /** The editor's JSON document, an opaque object checked against the schema `email_schema`
   * returns. It is rendered to HTML as a save would, then linted. A document that cannot be
   * rendered is an error finding with the reason. */
  doc?: Record<unknown>;
}

/** What the lint found in a message */
model LintEmailResult {
  /** What blocks a save and a send: the message cannot render. Empty when it can */
  errors: LintFinding[];

  /** Everything else; none of it blocks anything */
  warnings: LintFinding[];
}

@route("/emails/lint")
@tag("Emails")
namespace EmailLint {
  /**
   * Lint a message before a person sees it: the same lint that runs when a message is saved
   * and when a send is planned, plus warnings about a missing image description, low contrast,
   * and a color or font outside the Workspace's Brand kit. Nothing is saved. The message's
   * text and the Brand kit are untrusted data in the result. Send a `body` or a `doc`, not both
   * and not neither (422).
   */
  @post
  op lint(@body body: LintEmailInput):
    | LintEmailResult
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | BadRequestProblem
    | UnprocessableEntityProblem;
}

/** One attribute of a node or mark in the editor's document */
model EmailSchemaAttribute {
  /** The value the editor gives the attribute when the document omits it */
  default?: unknown;

  /** The kind of value: `string`, `integer`, `url`, `css`, `alignment` ... */
  type: string;

  /** The only values accepted, when the attribute is a closed set */
  allowed?: string[];

  /** The rule the server checks, in words: required or optional, range, form of the value */
  rule: string;
}

/** One node or mark type of the editor's document */
model EmailSchemaType {
  /** The content expression: which child nodes the node holds; absent for a leaf and for a mark */
  content?: string;

  /** The group a node belongs to (`block`, `inline`); absent for a mark */
  group?: string;

  /** The attributes the type accepts, by name. Any other attribute is refused. */
  attrs: Record<EmailSchemaAttribute>;
}

/** The editor's document format: what an agent writes as `doc` */
model EmailSchemaResult {
  /** Version of the editor whose export code renders a document */
  editorVersion: string;

  /** Node types by name. A document is `{"type":"doc","content":[...nodes]}` */
  nodes: Record<EmailSchemaType>;

  /** Mark types by name (bold, link ...), put in a text node's `marks` */
  marks: Record<EmailSchemaType>;

  /** Rules that hold across the document, in words */
  rules: string[];

  /** A valid document to start from */
  example: Record<unknown>;
}

@route("/emails/schema")
@tag("Emails")
namespace EmailSchema {
  /**
   * Read the schema of the editor's document: the node and mark types, the attributes each takes and
   * the values allowed. The structure comes from the editor itself; the attribute types and allowed
   * values are the rules the server checks, so a document that follows this is not refused. Write
   * the document as `doc` on a Template, Broadcast or Automation email step.
   */
  @get
  op get(): EmailSchemaResult | UnauthorizedProblem | TooManyRequestsProblem;
}

@route("/emails")
@tag("Emails")
namespace Emails {
  /**
   * Send a transactional email. The referenced Template's current content is
   * rendered with the supplied variables at send time. Respects the workspace
   * Suppression list (a suppressed destination returns status `suppressed`) but
   * not marketing Unsubscribe — transactional mail carries no sending source.
   *
   * Supply an `Idempotency-Key` to make retries safe: a repeated key replays the
   * original result without sending a second email, and a key whose first request
   * is still in flight returns 409.
   */
  @post
  op send(
    @body body: SendTransactionalEmailInput,

    /** Client-supplied key that makes a retried send idempotent (Stripe-style) */
    @header("Idempotency-Key") idempotencyKey?: string,
  ):
    | {
        @statusCode statusCode: 202;
        @body body: SendTransactionalEmailResponse;
      }
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | ConflictProblem
    | UnprocessableEntityProblem;
}
