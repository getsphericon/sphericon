using TypeSpec.Http;
using TypeSpec.OpenAPI;

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

/** The Workspace the token acts in */
model WorkspaceResource {
  /** The Workspace id: its short opaque public address */
  id: WorkspaceId;

  /** Display name */
  @untrusted
  name: string;

  /** The IANA time zone every analytics report counts its days in (ADR 0057) */
  timezone: TimeZoneName;

  /** The Send cap the platform set on the Workspace, with its sends over the last 24 hours; null
   * when no Send cap applies */
  sendCap?: SendCap | null;
}

@route("/workspace")
@tag("Workspace")
namespace Workspace {
  /** Read the Workspace the token acts in: its name, time zone and Send cap. Read the Send cap to
   * explain a slow send or to plan one: a send larger than what remains of it in the last 24 hours
   * goes out over more than a day. Read-only: no operation changes the Send cap. */
  @get
  @requiresScopes(#[])
  op get(): WorkspaceResource | UnauthorizedProblem | TooManyRequestsProblem;
}
