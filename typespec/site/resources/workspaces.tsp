using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.OpenAPI;

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

  /** Where an Owner or Admin can resolve the hold, as the setter gave it; absent when none was given */
  actionUrl?: string | null;
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

  /** The Workspace time zone (ADR 0057): the IANA name every analytics report counts its days
   * in. UTC until an Owner or Admin sets it. */
  timezone: TimeZoneName;

  /** The authenticated User's role in this Workspace (from their Membership). It
   * rides the Workspace list so role-gated UI is known even while the Workspace's
   * own endpoints are withheld under a Two-factor requirement. */
  role: SiteMembershipRole;

  /** The Permissions the authenticated User's Role holds in this Workspace (ADR 0061):
   * the client asks these whether to offer an action, it never computes them from the
   * Role. */
  permissions: Permission[];

  /** When outbound sending was suspended (ADR 0007); absent while the workspace can
   * send. A suspension freezes every send surface but not login, reads or tracking. */
  suspendedAt?: Timestamp | null;

  /** Why sending was suspended, shown to the owner; present only while suspended. */
  suspensionReason?: string | null;

  /** Where to appeal a suspension by email (SPHERICON_SUPPORT_EMAIL); absent when the instance
   * configured none. */
  supportEmail?: string | null;

  /** The public origin an AI agent connects to as `<mcpUrl>/mcp`: SPHERICON_MCP_URL when MCP has its
   * own host, otherwise the app origin. */
  mcpUrl: string;

  /** The public origin the tracker snippet loads `/t.js` from and posts `/collect` to:
   * SPHERICON_TRACKER_URL when the tracker has its own host, otherwise the app origin. */
  trackerUrl: string;

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

/** Create a Workspace (ADR 0043) */
model SiteCreateWorkspaceInput {
  /** Display name: free text, not unique */
  name: string;
}

/** Delete a Workspace (ADR 0043): the typed name must equal the stored name. */
model SiteDeleteWorkspaceInput {
  /** The Workspace's name, typed to confirm the deletion */
  name: string;
}

/** A Workspace in its deletion window, as an Owner of it sees it (ADR 0043): frozen and
 * hidden, restorable until its data is erased. */
model SiteDeletedWorkspaceResource {
  /** The Workspace id */
  id: WorkspaceId;

  /** Display name */
  name: string;

  /** When the Workspace was deleted */
  discardedAt: Timestamp;

  /** When its data will be erased: 30 days after it was deleted */
  eraseAt: Timestamp;

  /** Whether the caller is its only Owner: it then has no one to restore it once the
   * caller's User is deleted (ADR 0050) */
  onlyOwner: boolean;
}

/** Set the Workspace time zone */
model SiteWorkspaceTimezoneInput {
  /** An IANA time zone name such as `Europe/Berlin`; the zone database decides what is valid */
  timezone: TimeZoneName;
}

/** A whole number of days the Attribution window lasts: 1 to 90 (ADR 0064) */
@minValue(1)
@maxValue(90)
scalar AttributionWindowDays extends int32;

/** The Workspace's revenue settings (ADR 0064): the Reporting currency every revenue figure is
 * in, and the Attribution window an Order may follow a click by. Every member reads them. */
model SiteRevenueSettingsResource {
  /** The Reporting currency; null until an Owner or Admin chooses one, and revenue is not shown
   * before then */
  reportingCurrency: CurrencyCode | null;

  /** The Attribution window in whole days: 5 until set. A change affects only Orders that
   * arrive afterwards. */
  attributionWindowDays: AttributionWindowDays;
}

/** Change the revenue settings; a field left out stays as it is */
model SiteUpdateRevenueSettingsInput {
  /** The Reporting currency, an ISO 4217 code */
  reportingCurrency?: CurrencyCode;

  /** The Attribution window in whole days */
  attributionWindowDays?: AttributionWindowDays;
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

  /** Create a Workspace; the authenticated User becomes its Owner. Names are free text
   * and need not be unique. An Unverified User is refused with 403 and code
   * `email_verification_required`. The creation is recorded as an Audit entry. */
  @post
  op create(@body body: SiteCreateWorkspaceInput):
    | {
        @statusCode statusCode: 201;
        @body body: SiteWorkspaceResource;
      }
    | BadRequestProblem
    | ForbiddenProblem
    | UnprocessableEntityProblem;

  /** List the Workspaces in their deletion window that the authenticated User owns, with
   * the date their data will be erased. Answered while an acceptance is pending, for the
   * account deletion form (ADR 0062). */
  @get
  @route("/deleted")
  @allowedWhileAcceptancePending
  op listDeleted(): {
    @statusCode statusCode: 200;
    @body body: SiteDeletedWorkspaceResource[];
  };

  /** Delete a Workspace (Owner only). The typed name must equal the Workspace's name,
   * else 422. The Workspace is frozen at once and hidden: it answers 404 on its paths
   * and leaves every Member's list; an Owner restores it within 30 days. Recorded as an
   * Audit entry. Answered while an acceptance is pending (ADR 0062), so a User who solely owns
   * a Workspace can delete it and then their account; the Two-factor requirement still
   * applies. */
  @post
  @route("/{workspaceId}/delete")
  @requiresPermissions(#[Permission.workspaceDelete])
  @allowedWhileAcceptancePending
  op delete(
    @path workspaceId: WorkspaceId,
    @body body: SiteDeleteWorkspaceInput,
  ):
    | SiteDeletedWorkspaceResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Restore a Workspace in its deletion window (Owner only). It removes only the
   * pending-deletion freeze: a Workspace suspension or Staff hold stays. A deleted
   * Workspace is not exposed to anyone but its Owners, so a caller who is not an Owner
   * gets 404 as for an unknown one. A Workspace past its window, or already being
   * erased, answers 409; an erased one has no Owner left to ask, so it answers 404.
   * Recorded as an Audit entry. */
  @post
  @route("/deleted/{id}/restore")
  op restore(@path id: WorkspaceId):
    | SiteWorkspaceResource
    | BadRequestProblem
    | NotFoundProblem
    | ConflictProblem;

  /** Leave a Workspace (any Role): the caller's own Membership is removed at once, with no
   * Owner involved (Leaving, ADR 0050). 422 when the caller is the only Owner: another
   * Member must become Owner, or the Workspace be deleted, first. Recorded as an Audit
   * entry with the caller as actor. */
  @post
  @route("/{workspaceId}/leave")
  @requiresPermissions(#[])
  op leave(@path workspaceId: WorkspaceId):
    | NoContentResponse
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Rename a workspace or change its postal address (Owner and Admin only). The id
   * never changes. */
  @put
  @route("/{workspaceId}")
  @requiresPermissions(#[Permission.workspaceUpdate])
  op update(
    @path workspaceId: WorkspaceId,
    @body body: SiteUpdateWorkspaceInput,
  ):
    | SiteWorkspaceResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Switch the Two-factor requirement on (owner and admin only): every User with a
   * Membership must then have a Second factor. Switching it on while it is already on keeps
   * the original start, so no one's grace restarts. A change is recorded as an Audit entry. */
  @put
  @route("/{workspaceId}/second-factor-requirement")
  @requiresPermissions(#[Permission.secondFactorManage])
  op requireSecondFactor(@path workspaceId: WorkspaceId):
    | SiteWorkspaceResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem;

  /** Switch the Two-factor requirement off (owner and admin only). It is the one operation a
   * User withheld by the requirement may still perform in the Workspace, so an Owner or Admin
   * without a Second factor is never locked out of lifting it (ADR 0020). A change is recorded
   * as an Audit entry. */
  @delete
  @route("/{workspaceId}/second-factor-requirement")
  @allowedWhileWithheld
  @requiresPermissions(#[Permission.secondFactorManage])
  op liftSecondFactorRequirement(@path workspaceId: WorkspaceId):
    | SiteWorkspaceResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem;

  /** Set the Workspace time zone (Owner and Admin only), the zone of every Report day (ADR 0057). 422 for
   * a name the zone database does not know. A change is recorded as an Audit entry and applies
   * to all history at once. */
  @put
  @route("/{workspaceId}/timezone")
  @requiresPermissions(#[Permission.workspaceTimezone])
  op setTimezone(
    @path workspaceId: WorkspaceId,
    @body body: SiteWorkspaceTimezoneInput,
  ):
    | SiteWorkspaceResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Mint an Account link token (Owner only). Answers 404 when the installation has no
   * Account link configured. */
  @post
  @route("/{workspaceId}/account-link")
  @requiresPermissions(#[Permission.accountLinkOpen])
  op accountLink(@path workspaceId: WorkspaceId):
    | SiteAccountLinkResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem;

  /** Read the Workspace's revenue settings: the Reporting currency (null until chosen) and the
   * Attribution window. Open to every member. */
  @get
  @route("/{workspaceId}/revenue-settings")
  @requiresPermissions(#[])
  op getRevenueSettings(@path workspaceId: WorkspaceId):
    | SiteRevenueSettingsResource
    | BadRequestProblem
    | NotFoundProblem;

  /** Change the Reporting currency and the Attribution window (Owner and Admin only, Permission
   * `revenue:configure`, ADR 0061). 422 for a currency code that is not in ISO 4217 or a window
   * outside its bounds. Every change is recorded as an Audit entry naming who changed what. */
  @put
  @route("/{workspaceId}/revenue-settings")
  @requiresPermissions(#[Permission.revenueConfigure])
  op updateRevenueSettings(
    @path workspaceId: WorkspaceId,
    @body body: SiteUpdateRevenueSettingsInput,
  ):
    | SiteRevenueSettingsResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;
}
