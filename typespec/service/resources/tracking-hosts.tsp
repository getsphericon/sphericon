import "../../common/cursor-pages.tsp";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** What a Tracking domain host did at one change */
enum ServiceTrackingHostState {
  /** The host's CNAME reaches the edge: the edge may provision a certificate for it */
  dnsVerified: "dns_verified",

  /** The host answers over HTTPS: the Workspace's mail and tracker may use it */
  serving: "serving",

  /** The host left dns_verified and serving: tear it down. A host leaves when its record
   * stops resolving or when its Tracking domain goes (removed before it served, cancelled,
   * forgotten, or retired and its CNAME gone, ADR 0071). A retired host is still served. */
  removed: "removed",
}

/** One change of a Tracking domain host */
model ServiceTrackingHostChange {
  /** The Tracking domain's host */
  host: string;

  /** The Workspace that held the host at this change */
  workspaceId: WorkspaceId;

  /** What the host became; `removed` is the explicit report that it left the verified
   * states. Entries of one host arrive in the order they happened, so the last one read
   * is its current state. */
  state: ServiceTrackingHostState;

  /** When the change was recorded */
  changedAt: Timestamp;
}

/** One page of the Tracking host feed */
model ServiceTrackingHostFeedPage is FeedPage<
  ServiceTrackingHostChange,
  "Changes after the requested cursor, in the order they were made"
>;

@tag("Service Domains")
@route("/domains/changes")
namespace ServiceTrackingHostFeed {
  /** The changes of Tracking domain hosts since an opaque cursor, in the order they were
   * made: a host entering `dns_verified` or `serving`, moving between them, or leaving
   * them, which is reported as `removed` and never as silence. A caller that applies each
   * entry in order (provision on `dns_verified` and `serving`, tear down on `removed`)
   * mirrors the Instance's verified hosts, and a caller that starts from an empty cursor
   * rebuilds the whole set. Delivery is at-least-once: replaying a cursor returns the same
   * entries, so applying one twice must change nothing. */
  @get
  @requiresServiceScopes(#[ServiceScope.domainsRead])
  op list(...FeedQuery): ServiceTrackingHostFeedPage | Problem;
}
