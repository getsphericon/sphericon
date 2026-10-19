using TypeSpec.Http;

namespace Sphericon;

/** What an MCP client (dynamically registered) is asking permission for, as
 * validated by the authorization server. Rendered on the consent screen. */
model SiteOAuthAuthorizationRequest {
  /** Self-declared client name (untrusted: show as text) */
  clientName: string;

  /** Host of the client's metadata document URL when its client_id is one; absent for a registered client */
  clientHost?: string;

  /** The URL the user is sent back to */
  redirectUri: string;

  /** The redirect URI is on a loopback host: any program on the user's machine can be listening there */
  localRedirect: boolean;

  /** Scopes the client asked for that sphericon will grant by default */
  scopes: string[];

  /** Requested send-class scopes: granted only when the user opts in explicitly */
  sendScopes: string[];
}

/** The user's decision on an authorization request */
model SiteOAuthDecisionInput {
  clientId: string;
  redirectUri: string;

  /** Opaque client state, echoed back unchanged */
  state?: string;

  /** PKCE S256 challenge from the authorization request */
  codeChallenge: string;

  /** Space-delimited scopes from the authorization request */
  scope?: string;

  /** Workspace slug the token will belong to */
  workspaceSlug: string;

  /** true to approve, false to deny */
  approve: boolean;

  /** Opt in to the requested send-class scopes (default: not granted) */
  allowSend?: boolean;
}

/** Where to send the user's browser next (back to the client, with a code or an error) */
model SiteOAuthDecisionResult {
  redirectUrl: string;
}

@route("/oauth/authorization")
@tag("Site OAuth")
namespace SiteOAuth {
  /** Validate an OAuth authorization request for the consent screen */
  @get
  op describe(
    @query clientId: string,
    @query redirectUri: string,
    @query scope?: string,
  ):
    | {
        @statusCode statusCode: 200;
        @body body: SiteOAuthAuthorizationRequest;
      }
    | BadRequestProblem
    | NotFoundProblem;

  /** Approve or deny an OAuth authorization request as the signed-in user. Approving mints an API token, so it needs the same role as creating one (owner or admin). */
  @post
  op decide(@body body: SiteOAuthDecisionInput):
    | {
        @statusCode statusCode: 200;
        @body body: SiteOAuthDecisionResult;
      }
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem;
}
