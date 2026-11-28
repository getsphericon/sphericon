// The Sending domain models /site and /api show and take the same way. /site nests the resource
// under its Workspace with `@@parentResource` in its own resource file.

using TypeSpec.Rest;

namespace Sphericon;

/** Add a Sending domain; mints the DKIM keypair and returns the DNS records to publish. */
model CreateSendingDomainInput {
  /** The domain to authenticate, e.g. "mail.acme.com" (normalized to lowercase ASCII) */
  domain: string;

  /** DKIM selector; defaults to "sphericon" when omitted */
  dkimSelector?: string;
}

/** A TXT record the caller publishes: a Sending domain's DKIM key, or a Tracking domain's ownership */
model DnsRecord {
  /** The record type (always TXT for the records we generate) */
  type: "TXT";

  /** The host / name to create the record at */
  host: string;

  /** The record value to publish */
  value: string;
}

/**
 * Where an email Integration's envelope domain (MAIL FROM, Return-Path) is set for a Sending
 * domain, and so where SPF is checked for its mail (ADR 0184).
 */
enum SendingDomainEnvelopeState {
  /** SES sends the domain's mail with its own MAIL FROM domain: no custom one is set on the domain's identity */
  provider_default,

  /** SES sends the domain's mail with the custom MAIL FROM domain set on its identity (`mailFrom`) */
  custom,

  /** The identity could not be read: the Integration's credentials lack `permissionNeeded`, SES has no identity of the domain, the read timed out, or it has not been read yet */
  `unknown`,

  /** An SMTP relay: the envelope and its SPF are set at that provider, which the app does not know */
  set_at_provider,
}

/** SES's status of a custom MAIL FROM domain: whether it found the domain's MX record */
enum MailFromStatus {
  /** SES has not looked for the MX record yet */
  pending,

  /** SES found the MX record and sends with the custom domain */
  success,

  /** SES cannot find the MX record, or it no longer exists */
  failed,

  /** A passing issue kept SES from telling */
  temporary_failure,
}

/** What SES does with a send while the custom MAIL FROM domain's MX record fails */
enum MxFailureBehavior {
  /** SES sends with its own MAIL FROM domain instead */
  fall_back,

  /** SES refuses the send */
  reject,
}

/** The custom MAIL FROM domain set at SES on the Sending domain's identity */
model SendingDomainMailFrom {
  /** The custom MAIL FROM domain, e.g. "bounce.mail.acme.com" */
  domain: string;

  /** SES's status of it */
  status: MailFromStatus;

  /** What SES does with a send while its MX record fails */
  onMxFailure: MxFailureBehavior;
}

/**
 * The envelope one email Integration of the Workspace sends the Sending domain's mail with
 * (ADR 0184), read with the domain's checks. The app reads it; a custom MAIL FROM is set at SES by
 * the person, never by the app.
 */
model SendingDomainEnvelope {
  /** The email Integration */
  integrationId: EntityId;

  /** The Integration's name */
  integrationName: string;

  /** Its provider */
  provider: IntegrationProvider;

  /** Where the envelope is set */
  state: SendingDomainEnvelopeState;

  /** The custom MAIL FROM; null unless `state` is `custom` */
  mailFrom: SendingDomainMailFrom | null;

  /** The permission the Integration's credentials need to read the identity at SES; null unless `state` is `unknown` */
  permissionNeeded: string | null;

  /** When the envelope was last read; null for an SMTP relay and before the first read */
  checkedAt: Timestamp | null;
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

  /**
   * The envelope of each email Integration of the Workspace, the Test inbox excepted, in the order
   * of their ids (ADR 0184). SPF is checked on the envelope, not on the Sending domain, so the
   * domain has no SPF record of its own; it never gates sending.
   */
  envelopes: SendingDomainEnvelope[];

  /** The suggested DMARC TXT record (advisory bulk-readiness signal) */
  dmarcRecord: DnsRecord;

  /** When a DMARC lookup of the domain most recently answered, the record found or not (null = none has) */
  dmarcCheckedAt?: Timestamp | null;

  /**
   * When a lookup first found a DMARC record published for the domain, at the domain or a parent
   * of it (the DNS tree walk of RFC 9989) (null = the last lookup found none, or none has answered).
   * The domain is bulk-ready while it is set (ADR 0012); it never gates sending.
   */
  dmarcFoundAt?: Timestamp | null;

  /** When the DKIM DNS was most recently checked (null = never) */
  lastCheckedAt?: Timestamp | null;

  /** When the domain most recently became verified (null = never); kept after a loss */
  verifiedAt?: Timestamp | null;

  /** When a re-check most recently found the verified DKIM DNS gone (null = not lost: never verified, or verified again since) */
  verificationLostAt?: Timestamp | null;

  /** When the domain was deleted (discarded); null while it is kept. A deleted domain is no longer listed, is read-only, is never checked again and cannot be restored, but stays readable by id. */
  discardedAt?: Timestamp | null;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}
