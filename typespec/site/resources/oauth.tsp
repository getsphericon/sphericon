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
  scopes: GrantableScope[];

  /** Requested send-class scopes: granted only when the user opts in explicitly */
  sendScopes: SendScope[];

  /** Requested setup scopes (Sending domains, Integrations): granted only when the user opts in explicitly, apart from the send class */
  setupScopes: SetupScope[];

  /** The Permission a User must hold in the chosen Workspace to grant a requested scope, keyed by the scope; a scope absent here is any member's to grant (ADR 0061). The consent screen offers only what the User may grant, and the server refuses the rest. */
  requiredPermissions: Record<Permission>;
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

  /** Id of the Workspace the Connection will belong to (the signed-in User must be a member) */
  workspaceId: WorkspaceId;

  /** true to approve, false to deny */
  approve: boolean;

  /** Opt in to the requested send-class scopes (default: not granted) */
  allowSend?: boolean;

  /** Opt in to the requested setup scopes (default: not granted); independent of `allowSend` */
  allowSetup?: boolean;
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

  /** Approve or deny an OAuth authorization request as the signed-in user. Approving creates a Connection tied to the User's Membership, so any member may approve; a scope whose Permission the User's Role lacks is refused (403 permission_required). */
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
