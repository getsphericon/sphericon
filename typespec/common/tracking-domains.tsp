// The Tracking domain write body: /site and /api take the same input (ADR 0071).

namespace Sphericon;

/** Set the Workspace's Tracking domain */
model SetTrackingDomainInput {
  /** A host below a domain of your own, e.g. "t.example.com": never the apex (a CNAME cannot sit
   * there) nor one of the Instance's own hosts. Stored lower case, IDN as punycode. */
  host: string;
}
