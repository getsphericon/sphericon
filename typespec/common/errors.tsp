using TypeSpec.Http;

namespace Sphericon;

alias ValidationErrors = Record<string[]>;
alias FormValidationFields = Record<string>;

/** A machine-readable reason a client branches on, beyond the HTTP status */
enum ProblemCode {
  /** The Workspace requires a Second factor, the User has none and their grace
   * period is over (ADR 0020); enrolling one lifts the block */
  secondFactorRequired: "second_factor_required",

  /** The User's email is not proven yet (ADR 0030): login is refused until the
   * link sent to the address is followed */
  emailVerificationRequired: "email_verification_required",

  /** The caller's Role in the Workspace lacks the Permission the operation needs
   * (ADR 0061); another Role in the Workspace may perform it */
  permissionRequired: "permission_required",

  /** A development server's database has not run the migrations this build needs
   * (ADR 0033); `POST /site/schema/migrations` runs them. A production Instance never
   * sends it: its start guard refuses to start instead */
  migrationsPending: "migrations_pending",

  /** A required Sign-up item has a version the User has not accepted (ADR 0062): until they
   * accept it, only the few operations marked `x-allowed-while-acceptance-pending` answer them */
  acceptancePending: "acceptance_pending",
}

/** What a development server adds to a 500 so the page can show the fault; a production
 * Instance never sends it */
model ProblemDebug {
  /** The error that failed the request, or the panic value */
  error: string;

  /** The goroutine stack of a panic; absent for an error a handler returned */
  stack?: string;

  /** The request id the server log carries for this request */
  requestId?: string;

  /** The HTTP method of the failed request */
  method: string;

  /** The path of the failed request */
  path: string;
}

/** RFC 7807 Problem Details */
model ProblemDetails {
  /** Why the request was refused, when the client should act on it */
  code?: ProblemCode;

  /** A URI reference that identifies the problem type */
  type?: string;

  /** A short, human-readable summary of the problem type */
  title?: string;

  /** HTTP status code */
  status?: int32;

  /** A human-readable explanation specific to this occurrence */
  detail?: string;

  /** A URI reference that identifies the specific occurrence */
  instance?: string;

  /** Validation errors grouped by field */
  errors?: ValidationErrors;

  /** Form-level validation error */
  form?: string;

  /** Field validation errors */
  fields?: FormValidationFields;

  /** Seconds to wait before retrying; set on a 429 so a client that only sees the body can show the wait */
  retryAfter?: int32;

  /** Why a 500 happened, sent only by a development server (`SPHERICON_ENV=development`) */
  debug?: ProblemDebug;

  /** A removal refused because live work still uses the row (a 409, ADR 0049): the rows that
   * use it, never empty */
  usedBy?: UsedBy[];

  /** A refused User deletion (ADR 0050): the live Workspaces the User is the only Owner of, each
   * handed over or deleted first; empty when the refusal is another rule's */
  workspaces?: SoleOwnedWorkspace[];
}

/** The answer of every request a surface refuses or fails (RFC 7807), whatever its status: the
 * status is the response's and the body's `status`, and `code` tells apart the answers a client
 * branches on. A 429 (a rate limit, ADR 0025) and a 503 also carry `Retry-After` and, for the
 * rate limit, `X-RateLimit-Limit`, `X-RateLimit-Remaining` and `X-RateLimit-Reset`; the body's
 * `retryAfter` is the wait for a client that reads only the body. Every operation declares it,
 * as the `default` response (ADR 0103). */
@error
model Problem {
  @header contentType: "application/problem+json";
  @body body: ProblemDetails;
}

/** A Workspace the User is the only Owner of, which blocks User deletion */
model SoleOwnedWorkspace {
  /** The Workspace id */
  id: WorkspaceId;

  /** Display name */
  name: string;
}

/** What kind of row a refused removal names as a user of the row (ADR 0049). One value per kind
 * of live user a removal can find, so a client branches on it without parsing text. */
enum UsedByKind {
  /** A Broadcast that has not finished sending */
  broadcast,

  /** An Automation */
  automation,

  /** A Segment, through a rule that names a Tag */
  segment,

  /** An active Rule that uses the row, through a Condition leaf or an Action */
  rule,

  /** The active Enrollments of an Automation that is archived, as one entry carrying their count */
  enrollment,
}

/** A row that uses the one a removal was refused for: what a client shows to say what to
 * change first */
model UsedBy {
  /** What kind of row it is */
  kind: UsedByKind;

  /** Its id */
  id: EntityId;

  /** Its name */
  name: string;

  /** How many rows the entry stands for; absent means one. An `enrollment` entry names the
   * Automation (id, name) and counts its active Enrollments. */
  count?: int32;
}
