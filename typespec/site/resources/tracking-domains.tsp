using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** What a Tracking domain does for the Workspace (ADR 0071): `current` is used for new mail and the
 * snippet while it is `serving`; `replacement` takes over once it is `serving`; `retired` serves only
 * the links already sent and the snippet already installed on it, until its CNAME leaves the edge */
enum SiteTrackingDomainRole {
  current,
  replacement,
  retired,
}

/** State of a Tracking domain (ADR 0038): `pending` until the CNAME is found at the edge host, `dns_verified` once it is (the edge may provision), `serving` once HTTPS on the host answers */
enum SiteTrackingState {
  pending,
  dns_verified,
  serving,
}

/** Why a Tracking domain is held back although its CNAME may be right: `host_taken` means another Workspace already has this host verified (ADR 0038) */
enum SiteTrackingReason {
  host_taken: "host_taken",
}

/** The CNAME record to publish: `host` must point at `target` */
model SiteCnameRecord {
  /** The record type (always CNAME) */
  type: "CNAME";

  /** The host to create the record at */
  host: string;

  /** The edge host the CNAME must point at */
  target: string;
}

/** A Tracking domain of the Workspace (ADR 0071): a host of its own on which the links of its mail and the first-party tracker are served */
model SiteTrackingDomainResource {
  /** Unique identifier */
  id: EntityId;

  /** The host, lower case with IDN as punycode, e.g. "t.example.com" */
  host: string;

  /** What the host does for the Workspace */
  role: SiteTrackingDomainRole;

  /** Where the host's verification stands */
  state: SiteTrackingState;

  /** Why the host is not advancing; absent when nothing holds it back */
  reason?: SiteTrackingReason;

  /** The CNAME to publish */
  record: SiteCnameRecord;

  /** When the host first served (null = never): only a host that served is retired rather than deleted */
  servedAt?: Timestamp | null;

  /** When the host was most recently checked (null = never) */
  lastCheckedAt?: Timestamp | null;

  /** When the state most recently changed (null = never) */
  stateChangedAt?: Timestamp | null;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

@route("/workspaces/{workspaceId}/tracking-domains")
@tag("Site Tracking Domains")
namespace SiteTrackingDomains {
  /** List the Workspace's Tracking domains: the current one, the replacement, then the retired ones. Empty while the Instance offers none. */
  @get
  @requiresPermissions(#[])
  op list(@path workspaceId: WorkspaceId):
    | SiteTrackingDomainResource[]
    | BadRequestProblem
    | NotFoundProblem;

  /** Set the Workspace's Tracking domain (owner and admin only). With none it becomes the current one. While the current one serves it becomes the replacement (discarding a pending one) and takes over once it serves. When the current one does not serve, the new one replaces it at once. A host the Workspace already uses is returned as it is; a retired one is brought back. An invalid host or one of the Instance's own is 422; 404 when the Instance offers no Tracking domains. */
  @post
  @requiresPermissions(#[Permission.trackingDomainsManage])
  op set(@path workspaceId: WorkspaceId, @body body: SetTrackingDomainInput):
    | SiteTrackingDomainResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Check the host now (owner and admin only) and answer once the check is done: the Tracking domain as it left it. A replacement that reaches `serving` becomes current. */
  @post
  @route("/{id}/verify")
  @requiresPermissions(#[Permission.trackingDomainsManage])
  op verify(@path workspaceId: WorkspaceId, @path id: EntityId):
    | SiteTrackingDomainResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem;

  /** Remove a Tracking domain by its role (owner and admin only). The current one stops being used: it is retired when it served (links already sent on it keep working while its CNAME points at the edge), deleted otherwise, and a replacement becomes current. A replacement is cancelled. A retired one is forgotten: links sent on it stop working. */
  @delete
  @route("/{id}")
  @requiresPermissions(#[Permission.trackingDomainsManage])
  op remove(@path workspaceId: WorkspaceId, @path id: EntityId):
    | NoContentResponse
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem;
}
