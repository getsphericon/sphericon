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

/** State of a Tracking domain (ADR 0038): `pending` until the CNAME is found at the edge host, `dns_verified` once it is (the edge may provision), `serving` once HTTPS on the host answers; mail uses the host only while `serving` */
enum TrackingState {
  pending,
  dns_verified,
  serving,
}

/** Why a Tracking domain is held back although its CNAME may be right: `host_taken` means another Workspace already has this host verified (ADR 0038) */
enum TrackingReason {
  host_taken: "host_taken",
}

/** The Tracking domain CNAME of a Sending domain (ADR 0038): `host` must point at `target`. Read-only; absent when the instance has no tracking target configured. */
model TrackingRecord {
  /** The record type (always CNAME) */
  type: "CNAME";

  /** The host to create the record at: "track." followed by the domain */
  host: string;

  /** The edge host the CNAME must point at */
  target: string;

  /** Where the Tracking domain stands */
  state: TrackingState;

  /** Why the domain is not advancing; absent when nothing holds it back */
  reason?: TrackingReason;

  /** When the Tracking domain was most recently checked (null = never) */
  lastCheckedAt?: Timestamp | null;

  /** When the state most recently changed (null = never) */
  stateChangedAt?: Timestamp | null;
}

/**
 * Sending domain (ADR 0010). sphericon generates the DKIM keypair; the caller publishes the
 * DKIM TXT record. The private key is never returned. `verified` is a live property
 * re-checked in the background: it can flip back if the DNS record disappears.
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

  /** Whether the DKIM DNS is currently published and matches our key */
  verified: boolean;

  /** The required DKIM TXT record */
  dkimRecord: DnsRecord;

  /** The suggested SPF TXT record (advisory: does not gate sending) */
  spfRecord: DnsRecord;

  /** The suggested DMARC TXT record (advisory bulk-readiness signal) */
  dmarcRecord: DnsRecord;

  /** The Tracking domain CNAME; absent when the instance has no tracking target configured */
  trackingRecord?: TrackingRecord;

  /** When the DKIM DNS was most recently checked (null = never) */
  lastCheckedAt?: Timestamp | null;

  /** When the domain most recently became verified (null = never) */
  verifiedAt?: Timestamp | null;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Add a Sending domain; mints the DKIM keypair and returns the DNS records to publish. */
model CreateSendingDomainInput {
  /** The domain to authenticate, e.g. "mail.acme.com" (normalized to lowercase ASCII) */
  domain: string;

  /** DKIM selector; defaults to "sphericon" when omitted */
  dkimSelector?: string;
}

/** Change a Sending domain. The domain name is immutable; changing the selector moves the DKIM record, so the domain becomes unverified until the new record is published. */
model UpdateSendingDomainInput {
  /** New DKIM selector */
  dkimSelector?: string;
}

@tag("Sending domains")
interface SendingDomains
  extends ExternalCrudUnique<
      SendingDomainResource,
      CreateSendingDomainInput,
      UpdateSendingDomainInput,
      InUseProblem
    > {
  /** Trigger a live DKIM DNS check. The check runs in the background: the response is the domain as it stands now, 202 Accepted; read it again for the outcome (`verified`, `lastCheckedAt`). Verification is never set by the caller. */
  @action("verify")
  @autoRoute
  verify(...ResourceParameters<SendingDomainResource>):
    | {
        @statusCode statusCode: 202;
        @body body: SendingDomainResource;
      }
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem;
}
