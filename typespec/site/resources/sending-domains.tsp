using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** A DNS record the user publishes to authenticate a sending domain */
model SiteDnsRecord {
  /** The record type (always TXT for the records we generate) */
  type: "TXT";

  /** The host / name to create the record at */
  host: string;

  /** The record value to publish */
  value: string;
}

/**
 * Sending domain resource used by the site UI (ADR 0010). sphericon generates the
 * DKIM keypair; the user publishes the DKIM TXT to authenticate the domain. The
 * private key is never exposed. `verified` is re-checked in the background:
 * the domain loses it (`verificationLostAt`) if the DNS record disappears.
 */
@resource("sending-domains")
@segment("sending-domains")
@parentResource(SiteWorkspaceResource)
model SiteSendingDomainResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** The authenticated domain, e.g. "mail.acme.com" */
  domain: string;

  /** DKIM selector; the record host is "<selector>._domainkey.<domain>" */
  dkimSelector: string;

  /** Whether the domain is verified now: it has a `verifiedAt` and no `verificationLostAt` since */
  verified: boolean;

  /** The required DKIM TXT record (gates sending in a later slice) */
  dkimRecord: SiteDnsRecord;

  /** The suggested SPF TXT record (advisory — does not gate) */
  spfRecord: SiteDnsRecord;

  /** The suggested DMARC TXT record (advisory bulk-readiness signal) */
  dmarcRecord: SiteDnsRecord;

  /** When the DKIM DNS was most recently checked (null = never) */
  lastCheckedAt?: Timestamp | null;

  /** When the domain most recently became verified (null = never); kept after a loss */
  verifiedAt?: Timestamp | null;

  /** When a re-check most recently found the verified DKIM DNS gone (null = not lost: never verified, or verified again since) */
  verificationLostAt?: Timestamp | null;

  /** When the domain was archived; null while it is kept. */
  archivedAt?: Timestamp | null;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Filter of the sending domain list */
model SiteSendingDomainsListFilter {
  /** true lists the archived sending domains instead of the kept ones; the default hides them */
  @query archived?: boolean;
}

@tag("Site Sending Domains")
interface SiteSendingDomains {
  /** List sending domains for the site UI */
  @listsResource(SiteSendingDomainResource)
  @get
  @autoRoute
  @requiresPermissions(#[])
  list(
    ...ResourceCollectionParameters<SiteSendingDomainResource>,
    ...PageQuery,
    ...SiteSendingDomainsListFilter,
  ):
    | Page<SiteSendingDomainResource>
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Add a sending domain; mints the DKIM keypair and returns the DNS records. Owners and admins only. */
  @createsResource(SiteSendingDomainResource)
  @post
  @autoRoute
  @requiresPermissions(#[Permission.sendingDomainsManage])
  create(
    ...ResourceCollectionParameters<SiteSendingDomainResource>,
    @body body: CreateSendingDomainInput,
  ):
    | {
        @statusCode statusCode: 201;
        @body body: SiteSendingDomainResource;
      }
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem
    | ConflictProblem
    | UnprocessableEntityProblem;

  /** Get a sending domain by ID for the site UI */
  @readsResource(SiteSendingDomainResource)
  @get
  @autoRoute
  @requiresPermissions(#[])
  get(...ResourceParameters<SiteSendingDomainResource>):
    | SiteSendingDomainResource
    | BadRequestProblem
    | NotFoundProblem;

  /**
   * Archive a sending domain: it leaves the list and can be restored. Refused with 409 while a scheduled or sending broadcast or an active automation
   * sends from it. Owners and admins only.
   */
  @action("archive")
  @autoRoute
  @requiresPermissions(#[Permission.sendingDomainsManage])
  archive(...ResourceParameters<SiteSendingDomainResource>):
    | SiteSendingDomainResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem
    | InUseProblem;

  /**
   * Restore an archived sending domain. It starts a new DNS check. Refused with 409 when the same domain was added
   * again meanwhile. Owners and admins only.
   */
  @action("unarchive")
  @autoRoute
  @requiresPermissions(#[Permission.sendingDomainsManage])
  unarchive(...ResourceParameters<SiteSendingDomainResource>):
    | SiteSendingDomainResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem
    | ConflictProblem;

  /**
   * Re-check the DNS now and answer once the check is done: `verified` and `lastCheckedAt` of
   * the returned domain are the outcome. Owners and admins only.
   */
  @action("verify")
  @autoRoute
  @requiresPermissions(#[Permission.sendingDomainsManage])
  verify(...ResourceParameters<SiteSendingDomainResource>):
    | SiteSendingDomainResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem;
}
