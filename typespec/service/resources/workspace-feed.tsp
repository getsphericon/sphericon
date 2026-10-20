using TypeSpec.Http;

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

  /** Every owner of the Workspace as of this entry */
  owners: ServiceWorkspaceOwner[];
}

/** One page of the Workspace feed */
model ServiceWorkspaceFeedPage {
  /** Workspaces created or changed after the requested cursor, in change order */
  items: ServiceWorkspaceResource[];

  /** Pass it back as the cursor to read what follows this page. It is the requested
   * cursor itself when the page is empty. Saving it is what advances the caller: the
   * core keeps no position of its own. */
  nextCursor: string;

  /** Whether more entries follow this page right now */
  hasMore: boolean;
}

@tag("Service Workspaces")
@route("/workspaces/changes")
namespace ServiceWorkspaceFeed {
  /** The Workspaces created or changed since an opaque cursor, each with all of its
   * owners, in a stable order by change position. A change is a new Workspace, an owner
   * added, removed or transferred, or an owner's email or verification changing.
   * Delivery is at-least-once: replaying a cursor returns the same entries, and an
   * empty cursor starts from the beginning. Needs the `workspaces:read` scope. */
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
    | ServiceWorkspaceFeedPage
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem;
}
