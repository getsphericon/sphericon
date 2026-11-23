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

/** A suppressed destination: no send surface delivers to it */
model SuppressionResource {
  /** Unique identifier */
  id: EntityId;

  /** The Channel the suppression is on */
  channel: SuppressionChannel;

  /** Normalized destination: the lower-cased email address, the Telegram user id, or the phone number in E.164 */
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

  /** What to stop sending to: an email address on `email`, a Telegram user id (digits) on `telegram`, a phone number with its country code on `sms` (stored in E.164) */
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
    | Problem;
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
    | Problem;
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
   * texting START. Consent recorded before the number last withdrew it changes nothing and is
   * refused with a conflict.
   */
  @post
  @requiresScopes(#[ApiTokenScope.contactsWrite])
  op create(@body body: CreateConfirmationInput):
    | {
        @statusCode statusCode: 201;
        @body body: ConfirmationResource;
      }
    | Problem;
}

@route("/contacts/{contactId}/sms-consent")
@tag("Consent")
namespace SmsConsents {
  /**
   * Read whether a contact's phone may be texted: its SMS Confirmation (when and how the number
   * consented, with the wording, source and brand it carries) and its opt-out from every SMS of the
   * Workspace (when, and how the person stopped). Both are keyed by the number, so they follow the
   * phone the contact has now.
   */
  @get
  @requiresScopes(#[ApiTokenScope.contactsRead])
  op get(@path contactId: EntityId): SmsConsent | Problem;
}

/** An Engagement tracking consent the customer collected for a Destination (ADR 0120), recorded as `imported` */
model RecordEngagementTrackingInput {
  /** The Channel the consent is for. Only `email` is accepted yet */
  channel: EngagementTrackingChannel;

  /** The Destination that consented or refused: an email address on `email` */
  @maxLength(320)
  destination: string;

  ...EngagementTrackingObservationInput;
}

/** What became of an Engagement tracking observation: it set the consent (`applied`), or it occurred before the consent standing and changed nothing (`superseded`, recorded in the contact's history as `contact.consent_superseded`) */
enum EngagementTrackingOutcome {
  applied,
  superseded,
}

/** The answer to an Engagement tracking observation: what became of it, and the consent that stands after it */
model EngagementTrackingObservationResult {
  /** Whether the observation set the consent */
  outcome: EngagementTrackingOutcome;

  /** The Destination's consent after the observation */
  consent: EngagementTrackingConsent;
}

// A refusal of tracking applies from anyone; a grant applies only where no refusal stands, since
// only the person lifts a refusal (ADR 0121). There is deliberately no operation that lifts a
// `denied`: do not add one.
@tag("Consent")
namespace EngagementTracking {
  /**
   * Record whether a destination agreed to have its opens and clicks tracked, as an `imported`
   * consent collected outside sphericon (a consent banner, an account setting). A `denied` always
   * applies, and is kept in the immutable `email.engagement_tracking_denied` event; a `granted` is
   * kept in `email.engagement_tracking_granted` and is refused with a conflict over a standing
   * `denied`: only the person can lift a refusal. An observation that occurred before the consent
   * standing changes nothing and is answered as `superseded`; of two of the same moment the
   * refusal wins.
   */
  @route("/engagement-tracking-consents")
  @post
  @requiresScopes(#[ApiTokenScope.contactsWrite])
  op create(@body body: RecordEngagementTrackingInput):
    | EngagementTrackingObservationResult
    | Problem;

  /**
   * Read whether each Destination of a contact (its email address) is tracked: the consent
   * recorded with who recorded it, or `default` where none is, with what it means now. The consent
   * is keyed by the Destination, so it follows the address the contact has now.
   */
  @route("/contacts/{contactId}/engagement-tracking")
  @get
  @requiresScopes(#[ApiTokenScope.contactsRead])
  op getForContact(@path contactId: EntityId):
    | ContactEngagementTracking
    | Problem;
}
