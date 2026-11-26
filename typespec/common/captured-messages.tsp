// The Captured message models /site and /api show the same way.

namespace Sphericon;

/** The surface that asked for the send a Captured message belongs to */
enum CapturedMessageSurface {
  broadcast,
  automation,
  transactional,

  /** A confirmation email of double opt-in (ADR 0013) */
  confirmation,

  /** A Test send of a Broadcast: it has no Outbound message */
  test,
}
