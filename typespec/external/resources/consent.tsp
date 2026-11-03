using TypeSpec.Http;
using TypeSpec.OpenAPI;
using TypeSpec.Rest;

namespace Sphericon;

/** A Channel a destination can be suppressed or opt out on */
enum ConsentChannel {
  email,
  telegram,
}

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

  /** The Channel the suppression is on */
  channel: ConsentChannel;

  /** Normalized destination: the lower-cased email address, or the Telegram user id */
  destination: string;

  /** Why the destination is suppressed */
  reason: SuppressionReason;

  /** Creation timestamp */
  createdAt: Timestamp;
}

/** Request body for suppressing a destination */
model CreateSuppressionInput {
  /** The Channel to stop sending on; `email` when omitted */
  channel?: ConsentChannel;

  /** What to stop sending to: an email address on `email`, a Telegram user id (digits) on `telegram` */
  @maxLength(320)
  destination: string;
}

/** A per-sending-source opt-out of marketing */
model UnsubscribeResource {
  /** Unique identifier */
  id: EntityId;

  /** The Channel the opt-out is on */
  channel: ConsentChannel;

  /** Normalized destination: the lower-cased email address, or the Telegram user id */
  destination: string;

  /** The sending source opted out of: `broadcasts`, `automation:<id>` or `everything` */
  sendingSource: string;

  /** Creation timestamp */
  createdAt: Timestamp;
}

/** Request body for recording an unsubscribe */
model CreateUnsubscribeInput {
  /** The Channel the destination opted out on; `email` when omitted */
  channel?: ConsentChannel;

  /** What opted out: an email address on `email`, a Telegram user id (digits) on `telegram`. A Telegram opt-out covers every bot of the Workspace */
  @maxLength(320)
  destination: string;

  /** The sending source to opt out of: `broadcasts` (default), `automation:<id>`, or `everything` (leave entirely). On `telegram` only `everything` (the default) is accepted: a person stops all of the Workspace's Telegram messages */
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
  @requiresScopes(#[ApiTokenScope.suppressionsWrite])
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
  @requiresScopes(#[ApiTokenScope.contactsWrite])
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
