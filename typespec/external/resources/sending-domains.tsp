using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** A DNS record the caller publishes to authenticate a Sending domain */
model DnsRecord {
  /** The record type (always TXT for the records we generate) */
  type: "TXT";

  /** The host / name to create the record at */
  host: string;

  /** The record value to publish */
  value: string;
}

/**
 * Sending domain (ADR 0010). sphericon generates the DKIM keypair; the caller publishes the
 * DKIM TXT record. The private key is never returned. `verified` is re-checked in the
 * background: the domain loses it (`verificationLostAt`) if the DNS record disappears.
 */
@resource("sending-domains")
@segment("sending-domains")
model SendingDomainResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** The authenticated domain, e.g. "mail.acme.com" */
  domain: string;

  /** DKIM selector; the record host is "<selector>._domainkey.<domain>" */
  dkimSelector: string;

  /** Whether the domain is verified now: it has a `verifiedAt` and no `verificationLostAt` since */
  verified: boolean;

  /** The required DKIM TXT record */
  dkimRecord: DnsRecord;

  /** The suggested SPF TXT record (advisory: does not gate sending) */
  spfRecord: DnsRecord;

  /** The suggested DMARC TXT record (advisory bulk-readiness signal) */
  dmarcRecord: DnsRecord;

  /** When a DMARC lookup of the domain most recently answered, the record found or not (null = none has) */
  dmarcCheckedAt?: Timestamp | null;

  /**
   * When a lookup first found a DMARC record published for the domain, at the domain or its
   * organizational domain (null = the last lookup found none, or none has answered). The domain is
   * bulk-ready while it is set (ADR 0012); it never gates sending.
   */
  dmarcFoundAt?: Timestamp | null;

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

/**
 * Change a Sending domain. The domain name is immutable; changing the selector moves the DKIM
 * record, so the domain becomes unverified until the new record is published. The app has no
 * such edit yet, so this body is /api's own.
 */
model UpdateSendingDomainInput {
  /** New DKIM selector */
  dkimSelector?: string;
}

/** Filter of the Sending domain list */
model SendingDomainsListFilter {
  /** true lists the archived Sending domains instead of the kept ones; the default hides them */
  @query archived?: boolean;
}

@tag("Sending domains")
interface SendingDomains {
  /** List resources with pagination */
  @listsResource(SendingDomainResource)
  @get
  @autoRoute
  @requiresScopes(#[ApiTokenScope.sendingDomainsRead])
  list(
    ...ResourceCollectionParameters<SendingDomainResource>,
    ...PageQuery,
    ...SendingDomainsListFilter,
  ):
    | Page<SendingDomainResource>
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | UnprocessableEntityProblem;

  /** Create a resource */
  @createsResource(SendingDomainResource)
  @post
  @autoRoute
  @requiresScopes(#[ApiTokenScope.sendingDomainsWrite])
  create(
    ...ResourceCollectionParameters<SendingDomainResource>,
    @body body: CreateSendingDomainInput,
  ):
    | {
        @statusCode statusCode: 201;
        @body body: SendingDomainResource;
      }
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | ConflictProblem
    | UnprocessableEntityProblem;

  /** Get a resource by ID */
  @readsResource(SendingDomainResource)
  @get
  @autoRoute
  @requiresScopes(#[ApiTokenScope.sendingDomainsRead])
  get(...ResourceParameters<SendingDomainResource>):
    | SendingDomainResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  /** Update a resource */
  @updatesResource(SendingDomainResource)
  @put
  @autoRoute
  @requiresScopes(#[ApiTokenScope.sendingDomainsWrite])
  update(
    ...ResourceParameters<SendingDomainResource>,
    @body body: UpdateSendingDomainInput,
  ):
    | SendingDomainResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | ConflictProblem
    | UnprocessableEntityProblem;

  /** Archive a Sending domain: it leaves the list and can be restored. Refused with 409 (`usedBy`) while a scheduled or sending Broadcast or an active Automation sends from it. */
  @action("archive")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.sendingDomainsWrite])
  archive(...ResourceParameters<SendingDomainResource>):
    | SendingDomainResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | InUseProblem;

  /** Restore an archived Sending domain. It starts a new DNS check. Refused with 409 when the same domain was added again meanwhile. */
  @action("unarchive")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.sendingDomainsWrite])
  unarchive(...ResourceParameters<SendingDomainResource>):
    | SendingDomainResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | ConflictProblem;

  /** Run a live DNS check now and answer once it is done: the domain as the check left it (`verified`, `lastCheckedAt`). Verification is never set by the caller. */
  @action("verify")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.sendingDomainsWrite])
  verify(...ResourceParameters<SendingDomainResource>):
    | SendingDomainResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;
}
