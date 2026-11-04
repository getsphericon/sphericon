using TypeSpec.Http;
using TypeSpec.OpenAPI;
using TypeSpec.Rest;

namespace Sphericon;

/** A Channel a destination can opt out on or confirm on */
enum ConsentChannel {
  email,
  telegram,
  sms,
}

/** A Channel a destination can be suppressed on */
enum SuppressionChannel {
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
  channel: SuppressionChannel;

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
  channel?: SuppressionChannel;

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

  /** Normalized destination: the lower-cased email address, the Telegram user id, or the phone number in E.164 */
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

  /** What opted out: an email address on `email`, a Telegram user id (digits) on `telegram`, a phone number with its country code on `sms` (stored in E.164). A Telegram opt-out covers every bot of the Workspace; an SMS opt-out covers every sender of the Workspace */
  @maxLength(320)
  destination: string;

  /** The sending source to opt out of: `broadcasts` (default), `automation:<id>`, or `everything` (leave entirely). On `telegram` and `sms` only `everything` (the default) is accepted: a person stops all of the Workspace's messages on the Channel */
  sendingSource?: string;
}

/** A recorded consent of a destination on a Channel (ADR 0077): on `sms` a text goes only to a number that has one */
model ConfirmationResource {
  /** Unique identifier */
  id: EntityId;

  /** The Channel the confirmation is on */
  channel: ConsentChannel;

  /** Normalized destination: the phone number in E.164 on `sms` */
  destination: string;

  /** How the consent was obtained */
  provenance: ConfirmationProvenance;

  /** The consent wording the person was shown; absent when the confirmation carries no evidence */
  wording?: string | null;

  /** Where the consent was collected; absent when the confirmation carries no evidence */
  source?: string | null;

  /** The brand the person agreed to hear from; absent when the confirmation carries no evidence */
  brand?: string | null;

  /** When the consent was recorded */
  createdAt: Timestamp;
}

/** Request body for recording consent collected outside sphericon */
model CreateConfirmationInput {
  /** The Channel the consent is for. Only `sms` takes a recorded consent: email is confirmed by its own double opt-in link, and a Telegram chat is its own opt-in */
  channel: ConsentChannel;

  /** The phone number that consented, with its country code; stored in E.164 */
  @maxLength(320)
  destination: string;

  ...ConsentEvidence;
}

// Consent narrows through /api and MCP, and widens only by the evidence of consent the customer
// collected (ADR 0016, 0077): there is deliberately no operation to resubscribe, to lift an opt-out
// or to lift a suppression. Do not add one.
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
   * `broadcasts`), for an opt-out that reached you by any route. Idempotent. Resubscribing is not
   * available through the API.
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

@route("/confirmations")
@tag("Consent")
namespace Confirmations {
  /**
   * Record consent a phone number gave outside sphericon (a form of your own, a verification
   * service), as an `imported` confirmation with its evidence: the wording shown, where it was
   * collected and the brand. The evidence is kept in the immutable `sms.confirmed` event.
   * A number already confirmed keeps its confirmation unless the new one is stronger, so the
   * answer is the confirmation that stands. It does not lift an opt-out: only the person can, by
   * texting START.
   */
  @post
  @requiresScopes(#[ApiTokenScope.contactsWrite])
  op create(@body body: CreateConfirmationInput):
    | {
        @statusCode statusCode: 201;
        @body body: ConfirmationResource;
      }
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | UnprocessableEntityProblem;
}
