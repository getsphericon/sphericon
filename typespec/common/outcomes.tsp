// The closed lists of an Outbound send's Outcomes (ADR 0178): /site and /api show the same ones.

namespace Sphericon;

/** Why an Outbound message was Skipped: final for its Destination, never retried. The first four
 * are Send-eligibility's; a client switches over the list. */
enum SkippedReason {
  /** The destination is on the workspace's suppression list */
  suppressed,

  /** The destination unsubscribed from everything on the channel */
  unsubscribed_everything,

  /** The destination unsubscribed from the message's sending source */
  unsubscribed_source,

  /** The channel asks for a confirmation the destination does not have */
  unconfirmed,

  /** The contact has no destination on the channel: no address, no chat, no browser */
  no_destination,

  /** The contact was erased after the send was queued */
  contact_erased,

  /** No moment of the week is open in every time zone the recipient may be in, so the text cannot wait for its sending window */
  no_local_time,

  /** The destination has had as many marketing messages as its frequency cap allows in the period */
  frequency_cap,
}

/** What a Deferral waits for: the send is not consumed and goes through every check again when it is retried */
enum DeferralCause {
  /** The Provider limit of the Integration it sends through */
  provider_limit,

  /** The Workspace's Send cap, set by the platform */
  send_cap,

  /** The provider itself asked to slow down */
  provider,

  /** The recipient's sending window is closed: the text waits until it opens */
  sending_window,

  /** The destination is at its frequency cap: the send waits until a place frees */
  frequency_cap,
}

/** How many sends were Skipped for one reason */
model SkippedReasonCount {
  /** The reason, one of the closed list */
  reason: SkippedReason;

  /** Sends Skipped for it */
  count: int32;
}

/** How many sends wait now for one cause */
model DeferralCauseCount {
  /** The cause, one of the closed list */
  cause: DeferralCause;

  /** Sends whose latest Deferral was for it and that still wait */
  count: int32;
}
