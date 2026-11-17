// The Tracking domain models /site and /api show and take the same way (ADR 0071).

namespace Sphericon;

/** Set the Workspace's Tracking domain */
model SetTrackingDomainInput {
  /** A host below a domain of your own, e.g. "t.example.com": never the apex (a CNAME cannot sit
   * there) nor one of the Instance's own hosts. Stored lower case, IDN as punycode. */
  @minLength(1)
  host: string;
}

/** What a Tracking domain does for the Workspace (ADR 0071): `current` is used for new mail and the
 * snippet while it is `serving`; `replacement` takes over once it is `serving`; `retired` serves only
 * the links already sent and the snippet already installed on it, until its CNAME leaves the edge */
enum TrackingDomainRole {
  current,
  replacement,
  retired,
}

/** State of a Tracking domain (ADR 0038): `pending` until the CNAME is found at the edge host with the ownership TXT beside it, `dns_verified` once both are (the edge may provision), `serving` once HTTPS on the host answers */
enum TrackingState {
  pending,
  dns_verified,
  serving,
}

/** Why a Tracking domain is held back although its records may be right: `host_taken` means another Workspace already has this host verified (ADR 0038) */
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

  /** The TXT record to publish beside the CNAME, which shows the host is this Workspace's: every
   * check needs both (ADR 0173) */
  ownershipRecord: DnsRecord;

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
