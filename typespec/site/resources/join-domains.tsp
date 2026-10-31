using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** A Join domain: a domain whose Users may join the Workspace once domain join
 * is on, proven by a TXT record. `verified` is a live property: it holds while the record
 * is published. Any Workspace may claim a domain, and only one holds it verified. */
model SiteJoinDomainResource {
  /** Unique identifier */
  id: EntityId;

  /** The canonical domain: lower case, Unicode as punycode */
  domain: string;

  /** Whether the TXT record was found and the Workspace holds the domain */
  verified: boolean;

  /** The TXT record to publish to prove the domain */
  record: SiteDnsRecord;

  /** When the TXT record was most recently checked (null = never) */
  lastCheckedAt?: Timestamp | null;

  /** When the domain most recently became verified (null = pending) */
  verifiedAt?: Timestamp | null;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Request body for adding a Join domain */
model SiteAddJoinDomainInput {
  /** The domain, e.g. "acme.com"; any case or Unicode, stored canonical */
  domain: string;
}

@route("/workspaces/{workspaceId}/join-domains")
@tag("Site Join Domains")
namespace SiteJoinDomains {
  /** List the Workspace's Join domains (owner and admin only), pending and verified. */
  @get
  @requiresPermissions(#[Permission.joinDomainsManage])
  op list(@path workspaceId: WorkspaceId):
    | SiteJoinDomainResource[]
    | ForbiddenProblem
    | NotFoundProblem;

  /** Add a Join domain (owner and admin only). It starts pending, with the TXT record to publish at
   * `_sphericon-join.<domain>`. Another Workspace's claim never blocks it. An invalid
   * hostname is 422, a domain the Workspace already has is 409. */
  @post
  @requiresPermissions(#[Permission.joinDomainsManage])
  op add(@path workspaceId: WorkspaceId, @body body: SiteAddJoinDomainInput):
    | {
        @statusCode statusCode: 201;
        @body body: SiteJoinDomainResource;
      }
    | ForbiddenProblem
    | NotFoundProblem
    | ConflictProblem
    | UnprocessableEntityProblem;

  /** Check the TXT record now (owner and admin only) and verify the domain when it is published. A
   * domain another Workspace has verified is re-checked at that Workspace first: 409 while
   * its record is still published, otherwise the domain moves here. 422 when the record is
   * not found. */
  @post
  @route("/{id}/verify")
  @requiresPermissions(#[Permission.joinDomainsManage])
  op verify(@path workspaceId: WorkspaceId, @path id: EntityId):
    | SiteJoinDomainResource
    | ForbiddenProblem
    | NotFoundProblem
    | ConflictProblem
    | UnprocessableEntityProblem;

  /** Remove a Join domain (owner and admin only): only the claim goes, no Membership is touched. */
  @delete
  @route("/{id}")
  @requiresPermissions(#[Permission.joinDomainsManage])
  op remove(@path workspaceId: WorkspaceId, @path id: EntityId):
    | NoContentResponse
    | ForbiddenProblem
    | NotFoundProblem;
}

/** Whether the Workspace admits Users by Join domain */
model SiteDomainJoinResource {
  /** On: a User who proves an address on one of the Workspace's verified Join domains
   * becomes a member. Off by default. */
  enabled: boolean;
}

/** Switch domain join on or off */
model SiteSetDomainJoinInput {
  /** Whether to admit Users by Join domain from now on */
  enabled: boolean;
}

@route("/workspaces/{workspaceId}/domain-join")
@tag("Site Join Domains")
namespace SiteDomainJoin {
  /** Read the domain join switch (owner and admin only). */
  @get
  @requiresPermissions(#[Permission.joinDomainsManage])
  op get(@path workspaceId: WorkspaceId):
    | SiteDomainJoinResource
    | ForbiddenProblem
    | NotFoundProblem;

  /** Switch domain join on or off (owner and admin only). Switching it on adds nobody who is already
   * a User: only an address proven afterwards joins. Recorded as an Audit entry. */
  @put
  @requiresPermissions(#[Permission.joinDomainsManage])
  op set(@path workspaceId: WorkspaceId, @body body: SiteSetDomainJoinInput):
    | SiteDomainJoinResource
    | ForbiddenProblem
    | NotFoundProblem;
}
