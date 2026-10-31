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

  /** Email subject line */
  subject: string;

  /** Sender display name (defaults to the integration's) */
  fromName?: string | null;

  /** Sender email address (defaults to the integration's) */
  fromEmail?: EmailAddress | null;

  /** HTML body with Liquid merge tags */
  body: string;

  /** The editor's JSON document the body was built from; null when the body was written as HTML */
  doc?: Record<unknown> | null;

  /** Version of the editor that rendered the document; null with it */
  editorVersion?: string | null;

  /** Segment this broadcast goes to; null means all contacts. Once the broadcast leaves draft it holds a copy of the segment's condition taken then, and this names the segment only as where its audience came from. */
  segmentId?: EntityId | null;

  /** Lifecycle status. Broadcasts created here are always drafts. */
  status: BroadcastStatus;

  /** When the broadcast is scheduled to send */
  scheduledAt?: Timestamp | null;

  /** When the broadcast finished sending */
  sentAt?: Timestamp | null;

  /** When the broadcast was archived; null while it is kept. Only a sent or failed broadcast is archived. */
  archivedAt?: Timestamp | null;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Request body for setting a broadcast's Segment */
model SetBroadcastSegmentInput {
  /** The Segment to send to; null means all contacts */
  segmentId: EntityId | null;
}

/** Delivery report of a broadcast */
model BroadcastReport {
  /** Lifecycle status */
  status: BroadcastStatus;

  /**
   * Why sending is currently held, when it is: `workspace_suspended`,
   * `unverified_domain`, `no_integration` or `provider_stopped` (the provider has
   * stopped the account). A hold is reversible, not a failure.
   */
  holdReason?: string | null;

  /** Recipients the broadcast targets */
  recipientsTotal: int32;

  /** Messages handed to the email provider */
  sentCount: int32;

  /** Recipients skipped at send time (unsubscribed or suppressed after planning) */
  skippedCount: int32;

  /** Messages that failed to send */
  failedCount: int32;

  /** Recipients that opened the email */
  openedCount: int32;

  /** Recipients that clicked a link */
  clickedCount: int32;

  /** Recipients that unsubscribed via this broadcast */
  unsubscribedCount: int32;

  /** Messages the provider reported delivered, read from the delivery result stamped on each message */
  deliveredCount: int32;

  /** Messages the provider reported bounced, read from the delivery result stamped on each message */
  bouncedCount: int32;

  /** Messages a recipient complained about, read from the delivery result stamped on each message */
  complainedCount: int32;

  /** Messages that went through a Test inbox: captured, not delivered; they count as delivered too */
  capturedCount: int32;

  /** Unique opens ÷ messages sent (0-1) */
  openRate: float32;

  /** Unique clicks ÷ messages sent (0-1) */
  clickRate: float32;
}

/** Filter of the broadcast list */
model BroadcastsListFilter {
  /** true lists the archived broadcasts instead of the kept ones; the default hides them */
  @query archived?: boolean;
}

@tag("Broadcasts")
interface Broadcasts
  extends ExternalCrud<
      BroadcastResource,
      CreateBroadcastInput,
      UpdateBroadcastInput,
      ApiTokenScope.broadcastsRead,
      ApiTokenScope.broadcastsWrite,
      ApiTokenScope.broadcastsWrite,
      never,
      never,
      BroadcastsListFilter
    > {
  /** Archive a sent or failed broadcast: it leaves the list but keeps its report and figures, and can be restored. A draft is deleted instead; a scheduled or sending broadcast cannot be archived. */
  @action("archive")
  @autoRoute
  @extension("x-required-scopes", #[ApiTokenScope.broadcastsWrite])
  archive(...ResourceParameters<BroadcastResource>):
    | BroadcastResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Restore an archived broadcast to the list */
  @action("unarchive")
  @autoRoute
  @extension("x-required-scopes", #[ApiTokenScope.broadcastsWrite])
  unarchive(...ResourceParameters<BroadcastResource>):
    | BroadcastResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  /** Set the Segment of a draft broadcast, or null for all contacts. Does not send. */
  @action("segment")
  @put
  @autoRoute
  @extension("x-required-scopes", #[ApiTokenScope.broadcastsWrite])
  setSegment(
    ...ResourceParameters<BroadcastResource>,
    @body body: SetBroadcastSegmentInput,
  ):
    | BroadcastResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Send a rendered preview with sample merge data to a single address. Never sends to the audience. */
  @action("test-send")
  @autoRoute
  @extension("x-required-scopes", #[ApiTokenScope.broadcastsWrite])
  testSend(
    ...ResourceParameters<BroadcastResource>,
    @body body: TestSendBroadcastInput,
  ):
    | NoContentResponse
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Schedule a draft or scheduled broadcast to send to its audience at a future time. A draft copies its segment's condition: recipients are resolved at send time from that copy, so a later edit of the segment changes nothing for it, and rescheduling keeps it. An archived segment is refused. Send-class: needs the broadcasts:send scope, and mcp:send over MCP. */
  @action("schedule")
  @autoRoute
  @extension("x-required-scopes", #[ApiTokenScope.broadcastsSend])
  schedule(
    ...ResourceParameters<BroadcastResource>,
    @body body: ScheduleBroadcastInput,
  ):
    | BroadcastResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Cancel a scheduled broadcast and return it to draft, dropping the copy of its segment's condition: scheduling again copies the segment's current one. Send-class: needs the broadcasts:send scope, and mcp:send over MCP. */
  @action("unschedule")
  @autoRoute
  @extension("x-required-scopes", #[ApiTokenScope.broadcastsSend])
  unschedule(...ResourceParameters<BroadcastResource>):
    | BroadcastResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Read the delivery report: sent, skipped, failed, opened and clicked counts */
  @action("report")
  @get
  @autoRoute
  @extension("x-required-scopes", #[ApiTokenScope.broadcastsRead])
  report(...ResourceParameters<BroadcastResource>):
    | BroadcastReport
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;
}
