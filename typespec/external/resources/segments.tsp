using TypeSpec.Http;
using TypeSpec.OpenAPI;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

namespace Sphericon;

/** Segment resource */
@resource("segments")
@segment("segments")
model SegmentResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** Segment name */
  name: string;

  /** Segment condition: the tree of tests; an empty group matches all contacts */
  condition: SegmentCondition;

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
model PreviewSegmentInput {
  /** Condition; absent means all contacts */
  condition?: SegmentConditionInput;

  /**
   * The Channel of a Broadcast the audience is asked for (ADR 0149). When given, the result also
   * counts the recipients the Broadcast would take now.
   */
  channel?: BroadcastChannel;

  /** The Integration the Broadcast routes through; null or absent is the Channel's default. Only read with a channel */
  integrationId?: EntityId | null;
}

/** Result of a segment preview */
model PreviewSegmentResult {
  /** Number of contacts matching the condition, before Suppression and Unsubscribe */
  count: int32;

  /**
   * Number of contacts the condition leaves out because their engagement is unknown: it counts their
   * opens or clicks, and their Destination on that Channel is not tracked now (its Engagement
   * tracking consent, else the Workspace's default). They match neither the test nor its negation.
   */
  unknownEngagement: int32;

  /**
   * Number of matching contacts a Broadcast on the asked Channel would take now: those with a
   * Destination on it that is neither suppressed nor unsubscribed (on Telegram, an eligible Binding
   * on the routed Integration). The figure Schedule answers too. Absent unless a channel was given.
   */
  recipients?: int32;

  /**
   * How many days of analytical Events the Instance keeps (ADR 0019): present when an event test
   * counts with no window, or with one longer than that, since it counts only the Events kept.
   * Absent when every Event is kept, or no event test reaches past them.
   */
  eventsKeptDays?: int32;
}

@tag("Segments")
interface Segments
  extends ExternalCrudArchivable<
      SegmentResource,
      CreateSegmentInput,
      UpdateSegmentInput,
      ApiTokenScope.segmentsRead,
      ApiTokenScope.segmentsWrite
    > {
  /** Count the contacts matching an unsaved Condition, or all contacts without one. With a `channel` it also counts the recipients a Broadcast there would take now: read it back before a Broadcast is handed over for scheduling (ADR 0149). */
  @collectionAction(SegmentResource, "preview")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.segmentsRead])
  preview(
    ...ResourceCollectionParameters<SegmentResource>,
    @body body: PreviewSegmentInput,
  ): PreviewSegmentResult | Problem;
}
