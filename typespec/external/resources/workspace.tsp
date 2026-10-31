using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** The Send cap the platform set on the Workspace: the most messages it may send in any 24 hours,
 * across all its Integrations. Past it, marketing sends wait for the window to free up;
 * Transactional sends still go out and count. */
model SendCap {
  /** Most messages per rolling 24 hours */
  perDay: int32;

  /** Messages the Workspace sent in the last 24 hours, Transactional included */
  sentLast24h: int32;
}

/** The Workspace the API token belongs to */
model WorkspaceResource {
  /** The Workspace id */
  id: WorkspaceId;

  /** Display name */
  @untrusted
  name: string;

  /** The Send cap the platform set on the Workspace, with its last-24-hour sends; null when no
   * Send cap applies. A slow send past it is the Send cap, not the Provider limit. */
  sendCap: SendCap | null;
}

@route("/workspace")
@tag("Workspace")
namespace Workspace {
  /** Read the Workspace the token belongs to, with its Send cap: why a send may be slow and how
   * much of the day's sending is left. Read-only: no API call changes the Send cap. */
  @get
  @requiresScopes(#[])
  op get():
    | WorkspaceResource
    | UnauthorizedProblem
    | TooManyRequestsProblem;
}
