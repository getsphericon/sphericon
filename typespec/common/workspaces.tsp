// The Workspace models /site and /api show the same way.

namespace Sphericon;

/** The Workspace's Send cap (ADR 0067): the most messages it may send per rolling 24 hours across
 * all its Integrations, Transactional included. The platform sets it; the Workspace only reads it.
 * Sends beyond it are deferred until the 24-hour window frees up, never refused; Transactional mail
 * is never delayed. It is not a Provider limit, which each Integration sets for its own provider
 * account. */
model SendCap {
  /** Messages per rolling 24 hours */
  perDay: MaxPerDay;

  /** Messages the Workspace sent over the last 24 hours */
  sentLast24h: int32;
}
