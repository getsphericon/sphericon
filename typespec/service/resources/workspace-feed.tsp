import "../../common/cursor-pages.tsp";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** One owner of a Workspace in the feed */
model ServiceWorkspaceOwner {
  /** The owner's User id */
  userId: EntityId;

  /** The owner's current email */
  email: EmailAddress;

  /** When the owner confirmed their email; absent while it is unverified */
  emailVerifiedAt?: Timestamp | null;
}

/** A Workspace with all of its owners, as the feed reports it */
model ServiceWorkspaceResource {
  /** The Workspace id: its short opaque public address (ADR 0043) */
  id: WorkspaceId;

  name: string;

  /** When the Workspace was created */
  createdAt: Timestamp;

  /** When the Workspace was deleted; absent while it is live or after a restore. Its
   * 30-day window runs from here. */
  discardedAt?: Timestamp | null;

  /** When the erasure job removed the Workspace's data; absent until then. An erased
   * Workspace stays in the feed as a tombstone: its name is blank and it has no owners. */
  erasedAt?: Timestamp | null;

  /** Every owner of the Workspace as of this entry */
  owners: ServiceWorkspaceOwner[];
}

/** One page of the Workspace feed */
model ServiceWorkspaceFeedPage is FeedPage<
  ServiceWorkspaceResource,
  "Workspaces created or changed after the requested cursor, in change order"
>;

@tag("Service Workspaces")
@route("/workspaces/changes")
namespace ServiceWorkspaceFeed {
  /** The Workspaces created or changed since an opaque cursor, each with all of its
   * owners, in a stable order by change position. A change is a new Workspace, an owner
   * added, removed or transferred, or an owner's email or verification changing.
   * Delivery is at-least-once: replaying a cursor returns the same entries, and an
   * empty cursor starts from the beginning. */
  @get
  @requiresServiceScopes(#[ServiceScope.workspacesRead])
  op list(...FeedQuery): ServiceWorkspaceFeedPage | Problem;
}
