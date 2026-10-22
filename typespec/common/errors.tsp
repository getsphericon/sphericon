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

  /** The Instance closed public sign-up (ADR 0042): a new User comes only through an
   * Invitation or the Instance operator's `sphericon user create-owner` */
  signupClosed: "signup_closed",
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
}

/** RFC 7807 bad request response */
@error
model BadRequestProblem is Response<400> {
  @header contentType: "application/problem+json";
  @body body: ProblemDetails;
}

/** RFC 7807 gone response: the resource existed but is no longer available (an expired link) */
@error
model GoneProblem is Response<410> {
  @header contentType: "application/problem+json";
  @body body: ProblemDetails;
}

/** RFC 7807 unauthorized response */
@error
model UnauthorizedProblem is Response<401> {
  @header contentType: "application/problem+json";
  @body body: ProblemDetails;
}

/** RFC 7807 conflict response */
@error
model ConflictProblem is Response<409> {
  @header contentType: "application/problem+json";
  @body body: ProblemDetails;
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

  /** An active Rule with an Action that uses the row */
  rule,

  /** The Enrollments of an Automation, as one entry carrying their count: the active ones when
   * the Automation is archived, all of them when it is deleted */
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
   * Automation (id, name) and counts its Enrollments. */
  count?: int32;
}

/** The body of a refused removal (ADR 0049): ProblemDetails plus the rows that use it */
model InUseProblemDetails {
  ...ProblemDetails;

  /** The live rows that use the one a removal was refused for, never empty */
  usedBy: UsedBy[];
}

/** RFC 7807 conflict response for a removal refused because live work still uses the row
 * (ADR 0049): 409 with `usedBy` naming the users. Every refused removal of a Segment, Tag,
 * Integration, Sending domain or Automation answers it. */
@error
model InUseProblem is Response<409> {
  @header contentType: "application/problem+json";
  @body body: InUseProblemDetails;
}

/** RFC 7807 not found response */
@error
model NotFoundProblem is Response<404> {
  @header contentType: "application/problem+json";
  @body body: ProblemDetails;
}

/** RFC 7807 payment required response: the feature needs an Enterprise license */
@error
model PaymentRequiredProblem is Response<402> {
  @header contentType: "application/problem+json";
  @body body: ProblemDetails;
}

/** RFC 7807 forbidden response */
@error
model ForbiddenProblem is Response<403> {
  @header contentType: "application/problem+json";
  @body body: ProblemDetails;
}

/** RFC 7807 precondition required response: the request needs something it did not carry
 * (a verification link opened outside its Registration browser needs a password, ADR 0030) */
@error
model PreconditionRequiredProblem is Response<428> {
  @header contentType: "application/problem+json";
  @body body: ProblemDetails;
}

/** RFC 7807 too many requests response: a rate limit was exceeded (ADR 0025) */
@error
model TooManyRequestsProblem is Response<429> {
  @header contentType: "application/problem+json";

  /** Seconds to wait before retrying */
  @header("Retry-After") retryAfter: int32;

  /** The request budget of the window that was exceeded */
  @header("X-RateLimit-Limit") rateLimitLimit: int32;

  /** Requests left in the current window */
  @header("X-RateLimit-Remaining") rateLimitRemaining: int32;

  /** Unix time at which the window resets */
  @header("X-RateLimit-Reset") rateLimitReset: int64;

  @body body: ProblemDetails;
}

/** RFC 7807 validation response */
@error
model UnprocessableEntityProblem is Response<422> {
  @header contentType: "application/problem+json";
  @body body: ProblemDetails;
}
