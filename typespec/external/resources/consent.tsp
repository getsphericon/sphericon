using TypeSpec.Http;
using TypeSpec.OpenAPI;
using TypeSpec.Rest;

namespace Sphericon;

/** Why a destination is on the suppression (do-not-send) list */
enum SuppressionReason {
  bounce,
  complaint,
  manual,
}

/** A suppressed destination: no send surface delivers to it */
model SuppressionResource {
  /** Unique identifier */
  id: EntityId;

  /** Normalized (lower-cased) email address */
  destination: string;

  /** Why the destination is suppressed */
  reason: SuppressionReason;

  /** Creation timestamp */
  createdAt: Timestamp;
}

/** Request body for suppressing a destination */
model CreateSuppressionInput {
  /** Email address to stop sending to */
  destination: EmailAddress;
}

/** A per-sending-source opt-out of marketing */
model UnsubscribeResource {
  /** Unique identifier */
  id: EntityId;

  /** Normalized (lower-cased) email address */
  destination: string;

  /** The sending source opted out of: `broadcasts`, `automation:<id>` or `everything` */
  sendingSource: string;

  /** Creation timestamp */
  createdAt: Timestamp;
}

/** Request body for recording an unsubscribe */
model CreateUnsubscribeInput {
  /** Email address that opted out */
  destination: EmailAddress;

  /** The sending source to opt out of: `broadcasts` (default), `automation:<id>`, or `everything` (leave entirely) */
  sendingSource?: string;
}

// Consent only narrows through /api and MCP (ADR 0016): there is deliberately no
// operation to resubscribe or to lift a suppression. Do not add one.
@route("/suppressions")
@tag("Consent")
namespace Suppressions {
  /**
   * Suppress a destination so no surface sends to it (a manual Suppression).
   * Idempotent: an already-suppressed destination keeps its existing entry and
   * reason. Lifting a suppression is not available through the API.
   */
  // The suppressions:write scope needs suppressions:manage to be granted, and a
  // Connection's User holds it on every request, as on /site (ADR 0061).
  @post
  @extension("x-required-scopes", #[ApiTokenScope.suppressionsWrite])
  op create(@body body: CreateSuppressionInput):
    | {
        @statusCode statusCode: 201;
        @body body: SuppressionResource;
      }
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | UnprocessableEntityProblem;
}

@route("/unsubscribes")
@tag("Consent")
namespace Unsubscribes {
  /**
   * Record that a destination unsubscribed from a sending source (default
   * `broadcasts`). Idempotent. Resubscribing is not available through the API.
   */
  @post
  @extension("x-required-scopes", #[ApiTokenScope.contactsWrite])
  op create(@body body: CreateUnsubscribeInput):
    | {
        @statusCode statusCode: 201;
        @body body: UnsubscribeResource;
      }
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | UnprocessableEntityProblem;
}
