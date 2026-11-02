// Why a send is held: /site and /api show the same reasons.

namespace Sphericon;

/** Why a send is held (a Hold). A hold is reversible and not a failure: a held Broadcast keeps its
 * status and its remaining recipients send once it lifts, and an Automation's email steps wait. */
enum HoldReason {
  /** Sending is suspended for the workspace */
  workspace_suspended,

  /** The workspace is deleted and pending erasure */
  workspace_pending_deletion,

  /** Sending is on hold for the workspace */
  staff_hold,

  /** No email integration can send: the one named is disabled or archived, or there is no default */
  no_integration,

  /** The sender's domain is not a verified sending domain */
  unverified_domain,

  /** The email provider has stopped sending from the integration's account */
  provider_stopped,
}
