// Why and on which Channel a destination is suppressed: /site and /api show the same.

namespace Sphericon;

/** A Channel a destination can be suppressed on */
enum SuppressionChannel {
  email,
  telegram,
  sms,
}

/** Why a destination is on the suppression (do-not-send) list. An Unsubscribe is not a
 * suppression: it is a separate, per-source opt-out. */
enum SuppressionReason {
  bounce,
  complaint,
  manual,
}
