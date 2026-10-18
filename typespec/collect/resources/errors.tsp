using TypeSpec.Http;

namespace Sphericon;

/** RFC 7807 payload too large response: the body exceeds the size cap (ADR 0025) */
@error
model PayloadTooLargeProblem is Response<413> {
  @header contentType: "application/problem+json";
  @body body: ProblemDetails;
}

/** RFC 7807 service unavailable response: the database cannot be reached. The request changed nothing; retry it after `Retry-After` seconds with the same `clientEventId`s. */
@error
model ServiceUnavailableProblem is Response<503> {
  @header contentType: "application/problem+json";

  /** Seconds to wait before retrying */
  @header("Retry-After") retryAfter: int32;

  @body body: ProblemDetails;
}
