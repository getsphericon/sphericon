using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** What a Tracking domain does for the Workspace (ADR 0071): `current` is used for new mail and the
 * snippet while it is `serving`; `replacement` takes over once it is `serving`; `retired` serves only
 * the links already sent and the snippet already installed on it, until its CNAME leaves the edge */
enum TrackingDomainRole {
  current,
  replacement,
  retired,
}

/** State of a Tracking domain (ADR 0038): `pending` until the CNAME is found at the edge host, `dns_verified` once it is (the edge may provision), `serving` once HTTPS on the host answers */
enum TrackingState {
  pending,
  dns_verified,
  serving,
}

/** Why a Tracking domain is held back although its CNAME may be right: `host_taken` means another Workspace already has this host verified (ADR 0038) */
enum TrackingReason {
  host_taken: "host_taken",
}

/** The CNAME record to publish: `host` must point at `target` */
model CnameRecord {
  /** The record type (always CNAME) */
  type: "CNAME";

  /** The host to create the record at */
  host: string;

  /** The edge host the CNAME must point at */
  target: string;
}

/** A Tracking domain of the Workspace (ADR 0071): a host of its own on which the links of its mail and the first-party tracker are served */
model TrackingDomainResource {
  /** Unique identifier */
  id: EntityId;

  /** The host, lower case with IDN as punycode, e.g. "t.example.com" */
  host: string;

  /** What the host does for the Workspace */
  role: TrackingDomainRole;

  /** Where the host's verification stands */
  state: TrackingState;

  /** Why the host is not advancing; absent when nothing holds it back */
  reason?: TrackingReason;

  /** The CNAME to publish */
  record: CnameRecord;

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

@route("/tracking-domains")
@tag("Tracking domains")
namespace TrackingDomains {
  /** List the Workspace's Tracking domains: the current one, the replacement, then the retired ones. Empty while the Instance offers none. */
  @get
  @requiresScopes(#[ApiTokenScope.trackingDomainsRead])
  op list():
    | TrackingDomainResource[]
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem;

  /** Set the Workspace's Tracking domain. With none it becomes the current one. While the current one serves it becomes the replacement (discarding a pending one) and takes over once it serves. When the current one does not serve, the new one replaces it at once. A host the Workspace already uses is returned as it is; a retired one is brought back. An invalid host or one of the Instance's own is 422; 404 when the Instance offers no Tracking domains. */
  @post
  @requiresScopes(#[ApiTokenScope.trackingDomainsWrite])
  op set(@body body: SetTrackingDomainInput):
    | TrackingDomainResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Check the host now and answer once the check is done: the Tracking domain as it left it. A replacement that reaches `serving` becomes current. Verification is never set by the caller. */
  @post
  @route("/{id}/verify")
  @requiresScopes(#[ApiTokenScope.trackingDomainsWrite])
  op verify(@path id: EntityId):
    | TrackingDomainResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  /** Remove a Tracking domain by its role. The current one stops being used: it is retired when it served (links already sent on it keep working while its CNAME points at the edge), deleted otherwise, and a replacement becomes current. A replacement is cancelled. A retired one is forgotten: links sent on it stop working. */
  @delete
  @route("/{id}")
  @requiresScopes(#[ApiTokenScope.trackingDomainsWrite])
  op remove(@path id: EntityId):
    | NoContentResponse
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;
}
