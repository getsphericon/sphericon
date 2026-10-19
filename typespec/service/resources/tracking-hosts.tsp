using TypeSpec.Http;

namespace Sphericon;

/** What a Tracking domain host did at one change */
enum ServiceTrackingHostState {
  /** The host's CNAME reaches the edge: the edge may provision a certificate for it */
  dnsVerified: "dns_verified",

  /** The host answers over HTTPS: mail builds its links on it */
  serving: "serving",

  /** The host left dns_verified and serving: tear it down. A host leaves when its record
   * stops resolving or when its Sending domain is deleted or renamed. */
  removed: "removed",
}

/** One change of a Tracking domain host */
model ServiceTrackingHostChange {
  /** The host, `track.<Sending domain>` */
  host: string;

  /** The Workspace that held the host at this change */
  workspaceId: EntityId;

  /** What the host became; `removed` is the explicit report that it left the verified
   * states. Entries of one host arrive in the order they happened, so the last one read
   * is its current state. */
  state: ServiceTrackingHostState;

  /** When the change was recorded */
  changedAt: Timestamp;
}

/** One page of the Tracking host feed */
model ServiceTrackingHostFeedPage {
  /** Changes after the requested cursor, in the order they were made */
  items: ServiceTrackingHostChange[];

  /** Pass it back as the cursor to read what follows this page. It is the requested
   * cursor itself when the page is empty. Saving it is what advances the caller: the
   * core keeps no position of its own. */
  nextCursor: string;

  /** Whether more entries follow this page right now */
  hasMore: boolean;
}

@tag("Service Domains")
@route("/domains/changes")
namespace ServiceTrackingHostFeed {
  /** The changes of Tracking domain hosts since an opaque cursor, in the order they were
   * made: a host entering `dns_verified` or `serving`, moving between them, or leaving
   * them, which is reported as `removed` and never as silence. A caller that applies each
   * entry in order (provision on `dns_verified` and `serving`, tear down on `removed`)
   * mirrors the Instance's verified hosts, and a caller that starts from an empty cursor
   * rebuilds the whole set. Delivery is at-least-once: replaying a cursor returns the same
   * entries, so applying one twice must change nothing. Needs the `domains:read` scope. */
  @get
  op list(
    /** The `nextCursor` of a page the caller has saved; empty starts from the beginning */
    @query cursor?: string,

    /** Most entries on the page */
    @query
    @minValue(1)
    @maxValue(200)
    limit?: int32,
  ):
    | ServiceTrackingHostFeedPage
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem;
}
