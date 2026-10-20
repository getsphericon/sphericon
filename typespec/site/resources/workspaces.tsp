using TypeSpec.Http;
using TypeSpec.Rest;

namespace Sphericon;

/** A Staff hold on the Workspace's outbound sending (ADR 0009), as the Workspace's
 * members see it. It is not a suspension: it is set from outside, carries its own
 * message, and lifts by itself at its expiry. */
model SiteWorkspaceHold {
  /** The setter's key, unique per Workspace */
  key: string;

  /** Machine reason code the setter gave */
  reasonCode: string;

  /** The setter's message for the Owner, to show as given */
  message: string;

  /** When the hold was set */
  since: Timestamp;

  /** True for a billing hold (reason code `non_payment` or `free_plan_cap`): sending is paused for a money reason, not for abuse */
  billing: boolean;
}

/** Workspace the current user belongs to */
@resource("workspaces")
@segment("workspaces")
model SiteWorkspaceResource {
  /** The Workspace id: its short opaque public address and the route key for nested
   * workspace resources (ADR 0043) */
  @key("workspaceId")
  id: WorkspaceId;

  /** Display name */
  name: string;

  /** Per-workspace collect write-key (x-collect-key) for the tracking snippet */
  collectKey: string;

  /** Physical postal address printed in the marketing email footer (CAN-SPAM
   * 15 U.S.C. §7704(a)(5)). Empty until the workspace sets one. */
  postalAddress: string;

  /** The authenticated User's role in this Workspace (from their Membership). It
   * rides the Workspace list so role-gated UI is known even while the Workspace's
   * own endpoints are withheld under a Two-factor requirement. */
  role: SiteMembershipRole;

  /** When outbound sending was suspended (ADR 0007); absent while the workspace can
   * send. A suspension freezes every send surface but not login, reads or tracking. */
  suspendedAt?: Timestamp | null;

  /** Why sending was suspended, shown to the owner; present only while suspended. */
  suspensionReason?: string | null;

  /** Where to appeal a suspension by email (SPHERICON_SUPPORT_EMAIL); absent when the instance
   * configured none. */
  supportEmail?: string | null;

  /** The billing site a member under a hold is pointed to (SPHERICON_BILLING_URL); absent when the
   * instance configured none, and then no billing text is shown. */
  billingUrl?: string | null;

  /** The public origin an AI agent connects to as `<mcpUrl>/mcp`: SPHERICON_MCP_URL when MCP has its
   * own host, otherwise the app origin. */
  mcpUrl: string;

  /** The Staff holds active now, oldest key first; empty while nothing holds sending.
   * Any one freezes every send surface, like a suspension, but a lapsed hold is never
   * listed. */
  holds: SiteWorkspaceHold[];

  /** When an Owner or Admin switched on the Two-factor requirement (ADR 0020);
   * absent while every member may work without a Second factor. */
  secondFactorRequiredAt?: Timestamp | null;

  /** When the authenticated User's grace under the Two-factor requirement ends:
   * 7 days after the later of the requirement's start and their Membership's
   * creation. Absent without a requirement or once the User has a Second factor.
   * After it, every request to this Workspace answers 403 with code
   * second_factor_required until the User enrolls one. */
  secondFactorGraceEndsAt?: Timestamp | null;

  /** Whether the authenticated User may open the Account link: true only for an
   * Owner, and only on an installation configured for it. */
  accountLinkAvailable: boolean;

  /** Creation timestamp */
  createdAt: Timestamp;
}

/** A short-lived signed token that proves who the Owner is to the external account
 * page, with the address of that page. */
model SiteAccountLinkResource {
  /** Address of the external account page */
  url: string;

  /** Signed token (a JWT, EdDSA) naming the user, the Workspace and the role; it
   * expires within minutes. Verify it with the key set published at
   * /.well-known/jwks.json. */
  token: string;

  /** When the token expires */
  expiresAt: Timestamp;
}

/** Update a workspace. The id is immutable (it is the route key); the display name and the CAN-SPAM postal address can change. */
model SiteUpdateWorkspaceInput {
  /** New display name */
  name: string;

  /** Physical postal address for the marketing email footer (CAN-SPAM). Omit to
   * leave it unchanged; send an empty string to clear it. */
  postalAddress?: string;
}

/** Switch the Two-factor requirement on or off */
model SiteSecondFactorRequirementInput {
  /** Whether every User with a Membership must have a Second factor */
  required: boolean;
}

@route("/workspaces")
@tag("Site Workspaces")
namespace SiteWorkspaces {
  /** List workspaces owned by the authenticated user */
  @get
  op list(): {
    @statusCode statusCode: 200;
    @body body: SiteWorkspaceResource[];
  };

  /** Rename a workspace owned by the authenticated user */
  @put
  @route("/{workspaceId}")
  op update(
    @path workspaceId: WorkspaceId,
    @body body: SiteUpdateWorkspaceInput,
  ): SiteWorkspaceResource | NotFoundProblem | UnprocessableEntityProblem;

  /** Switch the Two-factor requirement on or off (owner and admin only). Switching
   * it on while it is already on keeps the original start, so no one's grace
   * restarts. A change is recorded as an Audit entry. */
  @put
  @route("/{workspaceId}/second-factor-requirement")
  op setSecondFactorRequirement(
    @path workspaceId: WorkspaceId,
    @body body: SiteSecondFactorRequirementInput,
  ): SiteWorkspaceResource | ForbiddenProblem | NotFoundProblem;

  /** Mint an Account link token (Owner only). Answers 404 when the installation has no
   * Account link configured. */
  @post
  @route("/{workspaceId}/account-link")
  op accountLink(@path workspaceId: WorkspaceId):
    | SiteAccountLinkResource
    | ForbiddenProblem
    | NotFoundProblem;
}
