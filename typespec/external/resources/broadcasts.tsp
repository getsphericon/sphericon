using TypeSpec.Http;
using TypeSpec.OpenAPI;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

namespace Sphericon;

/** Broadcast lifecycle status */
enum BroadcastStatus {
  draft,
  scheduled,
  sending,
  sent,
  failed,
}

/** Broadcast resource */
@resource("broadcasts")
@segment("broadcasts")
model BroadcastResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** Internal broadcast name */
  name: string;

  /** The Channel the broadcast sends on, fixed when it was created */
  channel: BroadcastChannel;

  /** What the broadcast says, in the shape of its Channel */
  content: BroadcastContent;

  /** Segment this broadcast goes to; null means all contacts. Once the broadcast leaves draft it holds a copy of the segment's condition taken then, and this names the segment only as where its audience came from. */
  segmentId?: EntityId | null;

  /** Lifecycle status. Broadcasts created here are always drafts. */
  status: BroadcastStatus;

  /** When the broadcast is scheduled to send */
  scheduledAt?: Timestamp | null;

  /** When the broadcast finished sending */
  sentAt?: Timestamp | null;

  /** When the broadcast failed */
  failedAt?: Timestamp | null;

  /** When the broadcast was archived; null while it is kept. An archived broadcast is read-only until it is unarchived. */
  archivedAt?: Timestamp | null;

  /** The revision of the state this answer shows (ADR 0119): an update, a change of its segment and Schedule name it */
  revision: Revision;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Request body for setting a broadcast's Segment */
model SetBroadcastSegmentInput {
  /** The broadcast's `revision` the change was made on (ADR 0119): the one it was read with */
  revision: Revision;

  /** The Segment to send to; null means all contacts */
  segmentId: EntityId | null;
}

/**
 * Delivery report of a broadcast, per Channel (ADR 0080). A figure the broadcast's Channel does
 * not report is null, never zero: Telegram reports no delivered, opened, bounced or complained
 * count and so no open rate; SMS reports delivered and undelivered, and no opened, bounced or
 * complained count or open rate; only SMS reports undelivered.
 */
model BroadcastReport {
  /** Lifecycle status */
  status: BroadcastStatus;

  /** Why sending is currently held, when it is */
  holdReason?: HoldReason | null;

  /** Recipients the broadcast targets */
  recipientsTotal: int32;

  /** Messages handed to the provider */
  sentCount: int32;

  /** Recipients skipped at send time (no destination any more, unsubscribed or suppressed after planning) */
  skippedCount: int32;

  /** Messages that failed to send */
  failedCount: int32;

  /** Recipients whose send has not ended yet */
  pendingCount: int32;

  /** Recipients that opened the email; null on a Channel that cannot tell */
  openedCount: int32 | null;

  /** Recipients that clicked a link */
  clickedCount: int32;

  /** Recipients that unsubscribed via this broadcast */
  unsubscribedCount: int32;

  /** Messages the provider reported delivered, read from the delivery result stamped on each message; null on a Channel whose provider reports none */
  deliveredCount: int32 | null;

  /** Messages the provider reported undelivered (an SMS the carrier could not deliver), read from the delivery result stamped on each message; null on a Channel whose provider reports none */
  undeliveredCount: int32 | null;

  /** Messages the provider reported bounced, read from the delivery result stamped on each message; null on a Channel whose provider reports none */
  bouncedCount: int32 | null;

  /** Messages a recipient complained about, read from the delivery result stamped on each message; null on a Channel whose provider reports none */
  complainedCount: int32 | null;

  /** Messages that went through a Test inbox: captured, not delivered; they count as delivered too. Null off email. */
  capturedCount: int32 | null;

  /** Unique opens ÷ messages sent (0-1); null on a Channel that reports no opens */
  openRate: float32 | null;

  /** Unique clicks ÷ messages sent (0-1) */
  clickRate: float32;
}

/** Filter of the broadcast list */
model BroadcastsListFilter {
  /** true lists the archived broadcasts instead of the kept ones; the default hides them */
  @query archived?: boolean;

  /** Only the broadcasts on this Channel; every Channel when absent */
  @query channel?: BroadcastChannel;
}

/** A broadcast is archived, never deleted (ADR 0100). An archived broadcast is read-only until it
 * is unarchived: an update, a change of its Segment or a schedule of one is a 409. */
@tag("Broadcasts")
interface Broadcasts
  extends ExternalCrudKept<
      BroadcastResource,
      CreateBroadcastInput,
      UpdateBroadcastInput,
      ApiTokenScope.broadcastsRead,
      ApiTokenScope.broadcastsWrite,
      BroadcastsListFilter
    > {
  /** Archive a draft, sent or failed broadcast: it leaves the list but keeps its report and figures, and can be unarchived. A scheduled or sending broadcast is work in flight and cannot be archived (409). Archiving an archived broadcast changes nothing. */
  @action("archive")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.broadcastsWrite])
  archive(...ResourceParameters<BroadcastResource>):
    | BroadcastResource
    | Problem;

  /** Bring an archived broadcast back to the list, in the status it had */
  @action("unarchive")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.broadcastsWrite])
  unarchive(...ResourceParameters<BroadcastResource>):
    | BroadcastResource
    | Problem;

  /** Set the Segment of a draft broadcast, or null for all contacts. Does not send. An archived broadcast is a 409. */
  @action("segment")
  @put
  @autoRoute
  @requiresScopes(#[ApiTokenScope.broadcastsWrite])
  setSegment(
    ...ResourceParameters<BroadcastResource>,
    @body body: SetBroadcastSegmentInput,
  ): BroadcastResource | Problem;

  /** Send a rendered preview with sample merge data to a single address: an email broadcast to the `email` given, an SMS broadcast to the `phone` given, through the broadcast's Integration. Never sends to the audience, and leaves no message and no figure. Telegram has no Test send (ADR 0080), so a Telegram broadcast is a 422. */
  @action("test-send")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.broadcastsWrite])
  testSend(
    ...ResourceParameters<BroadcastResource>,
    @body body: TestSendBroadcastInput,
  ): NoContentResponse | Problem;

  /** Schedule a draft or scheduled broadcast to send to its audience at a future time. A draft copies its segment's condition: recipients are resolved at send time from that copy, so a later edit of the segment changes nothing for it, and rescheduling keeps it. An archived segment is refused. 409 when the integration it sends through cannot send (disabled or archived, or no default): a draft may name such an integration, only scheduling it is refused, and so is an archived broadcast. Send-class: needs the broadcasts:send scope, and mcp:send over MCP. */
  @action("schedule")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.broadcastsSend])
  schedule(
    ...ResourceParameters<BroadcastResource>,
    @body body: ScheduleBroadcastInput,
  ): BroadcastResource | Problem;

  /** Cancel a scheduled broadcast and return it to draft, dropping the copy of its segment's condition: scheduling again copies the segment's current one. Send-class: needs the broadcasts:send scope, and mcp:send over MCP. */
  @action("unschedule")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.broadcastsSend])
  unschedule(...ResourceParameters<BroadcastResource>):
    | BroadcastResource
    | Problem;

  /** Read the delivery report: sent, skipped, failed, opened and clicked counts */
  @action("report")
  @get
  @autoRoute
  @requiresScopes(#[ApiTokenScope.broadcastsRead])
  report(...ResourceParameters<BroadcastResource>): BroadcastReport | Problem;
}
