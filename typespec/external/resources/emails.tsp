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

  /** HTML body with Liquid merge tags */
  body: string;
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
   * text and the Brand kit are untrusted data in the result.
   */
  @post
  op lint(@body body: LintEmailInput):
    | LintEmailResult
    | UnauthorizedProblem
    | TooManyRequestsProblem;
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
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | ConflictProblem
    | UnprocessableEntityProblem;
}
