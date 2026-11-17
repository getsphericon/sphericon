using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** Segment resource used by the site UI */
@resource("segments")
@segment("segments")
@parentResource(SiteWorkspaceResource)
model SiteSegmentResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** Segment name */
  name: string;

  /** Segment condition: the tree of tests; an empty group matches all contacts */
  condition?: ConditionGroup;

  /** When the segment was archived; null while it is kept */
  archivedAt?: Timestamp | null;

  /** The revision of the state this answer shows (ADR 0119): an update names it */
  revision: Revision;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Request body for previewing a condition's segment */
model SitePreviewSegmentInput {
  /** Condition; absent means all contacts */
  condition?: ConditionGroupInput;

  /**
   * The Channel of a Broadcast the audience is asked for. When given, the result also counts the
   * recipients the Broadcast would take: the matching contacts it could send to now.
   */
  channel?: BroadcastChannel;

  /** The Integration the Broadcast routes through; null or absent is the Channel's default. Only read with a channel */
  integrationId?: EntityId | null;
}

/** Result of a segment preview: how many deliverable contacts match */
model SitePreviewSegmentResult {
  /** Number of active contacts matching the condition */
  count: int32;

  /**
   * Number of matching contacts a Broadcast on the asked Channel would take: on email those with a
   * mailable address, on Telegram those with an eligible Binding on the routed Integration. Absent
   * unless a channel was given.
   */
  recipients?: int32;
}

@tag("Site Segments")
interface SiteSegments
  extends WorkspaceCrudArchivable<
      SiteSegmentResource,
      CreateSegmentInput,
      UpdateSegmentInput,
      #[],
      #[]
    > {
  /** Preview how many active contacts match a rule definition */
  @collectionAction(SiteSegmentResource, "preview")
  @autoRoute
  @requiresPermissions(#[])
  preview(
    ...ResourceCollectionParameters<SiteSegmentResource>,
    @body body: SitePreviewSegmentInput,
  ): SitePreviewSegmentResult | Problem;
}

@route("/workspaces/{workspaceId}/segments/{segmentId}/contacts")
@tag("Site Segments")
namespace SiteSegmentContacts {
  /**
   * List the contacts a segment matches right now, evaluated live, ascending by id. A stored
   * condition that no longer fits the contact schema is a 422.
   */
  @get
  @requiresPermissions(#[])
  op list(
    @path workspaceId: WorkspaceId,
    @path segmentId: EntityId,
    ...PageQuery,
  ): Page<SiteContactResource> | Problem;
}
