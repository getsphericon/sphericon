using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

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

/** State of a Tracking domain (ADR 0038): `pending` until the CNAME is found at the edge host, `dns_verified` once it is (the edge may provision), `serving` once HTTPS on the host answers; mail uses the host only while `serving` */
enum SiteTrackingState {
  pending,
  dns_verified,
  serving,
}

/** Why a Tracking domain is held back although its CNAME may be right: `host_taken` means another Workspace already has this host verified (ADR 0038) */
enum SiteTrackingReason {
  host_taken: "host_taken",
}

/** The Tracking domain CNAME of a Sending domain (ADR 0038): `host` must point at `target`. Read-only; absent when the instance has no tracking target configured. */
model SiteTrackingRecord {
  /** The record type (always CNAME) */
  type: "CNAME";

  /** The host to create the record at: "track." followed by the domain */
  host: string;

  /** The edge host the CNAME must point at */
  target: string;

  /** Where the Tracking domain stands */
  state: SiteTrackingState;

  /** Why the domain is not advancing; absent when nothing holds it back */
  reason?: SiteTrackingReason;

  /** When the Tracking domain was most recently checked (null = never) */
  lastCheckedAt?: Timestamp | null;

  /** When the state most recently changed (null = never) */
  stateChangedAt?: Timestamp | null;
}

/**
 * Sending domain resource used by the site UI (ADR 0010). sphericon generates the
 * DKIM keypair; the user publishes the DKIM TXT to authenticate the domain. The
 * private key is never exposed. `verified` is a live property re-checked in the
 * background — it can flip back if the DNS record disappears.
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

  /** Whether the DKIM DNS is currently published and matches our key */
  verified: boolean;

  /** The required DKIM TXT record (gates sending in a later slice) */
  dkimRecord: SiteDnsRecord;

  /** The suggested SPF TXT record (advisory — does not gate) */
  spfRecord: SiteDnsRecord;

  /** The suggested DMARC TXT record (advisory bulk-readiness signal) */
  dmarcRecord: SiteDnsRecord;

  /** The Tracking domain CNAME; absent when the instance has no tracking target configured */
  trackingRecord?: SiteTrackingRecord;

  /** When the DKIM DNS was most recently checked (null = never) */
  lastCheckedAt?: Timestamp | null;

  /** When the domain most recently became verified (null = never) */
  verifiedAt?: Timestamp | null;

  /** When the domain was archived; null while it is kept. An archived domain serves no Tracking domain host. */
  archivedAt?: Timestamp | null;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Site request body for adding a sending domain */
model SiteCreateSendingDomainInput {
  /** The domain to authenticate, e.g. "mail.acme.com" */
  domain: string;

  /** DKIM selector; defaults to "sphericon" when omitted */
  dkimSelector?: string;
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
  create(
    ...ResourceCollectionParameters<SiteSendingDomainResource>,
    @body body: SiteCreateSendingDomainInput,
  ):
    | {
        @statusCode statusCode: 201;
        @body body: SiteSendingDomainResource;
      }
    | ForbiddenProblem
    | NotFoundProblem
    | ConflictProblem
    | UnprocessableEntityProblem;

  /** Get a sending domain by ID for the site UI */
  @readsResource(SiteSendingDomainResource)
  @get
  @autoRoute
  get(...ResourceParameters<SiteSendingDomainResource>):
    | SiteSendingDomainResource
    | BadRequestProblem
    | NotFoundProblem;

  /**
   * Archive a sending domain: it leaves the list, serves no Tracking domain host and can be
   * restored. Refused with 409 while a scheduled or sending broadcast or an active automation
   * sends from it. Owners and admins only.
   */
  @action("archive")
  @autoRoute
  archive(...ResourceParameters<SiteSendingDomainResource>):
    | SiteSendingDomainResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem
    | InUseProblem;

  /**
   * Restore an archived sending domain. It starts a new DNS check and serves its Tracking
   * domain host again only once that passes. Refused with 409 when the same domain was added
   * again meanwhile. Owners and admins only.
   */
  @action("unarchive")
  @autoRoute
  unarchive(...ResourceParameters<SiteSendingDomainResource>):
    | SiteSendingDomainResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem
    | ConflictProblem;

  /** Trigger an immediate DKIM re-check (fire-and-forget). Owners and admins only. */
  @action("verify")
  @autoRoute
  verify(...ResourceParameters<SiteSendingDomainResource>):
    | NoContentResponse
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem;
}
