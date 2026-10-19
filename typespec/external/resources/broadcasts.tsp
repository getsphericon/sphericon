using TypeSpec.Http;
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

  /** Audience: the Segment this broadcast goes to; null means all active contacts */
  segmentId?: EntityId | null;

  /** Lifecycle status. Broadcasts created here are always drafts. */
  status: BroadcastStatus;

  /** When the broadcast is scheduled to send */
  scheduledAt?: Timestamp | null;

  /** When the broadcast finished sending */
  sentAt?: Timestamp | null;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Request body for creating a broadcast (always created as a draft) */
model CreateBroadcastInput {
  /** Internal broadcast name */
  name: string;

  /** Email subject line */
  subject?: string;

  /** Sender display name */
  fromName?: string;

  /** Sender email address */
  fromEmail?: EmailAddress;

  /** HTML body with Liquid merge tags */
  body?: string;
}

/** Request body for updating a draft broadcast */
model UpdateBroadcastInput {
  /** Internal broadcast name */
  name?: string;

  /** Email subject line */
  subject?: string;

  /** Sender display name */
  fromName?: string;

  /** Sender email address */
  fromEmail?: EmailAddress;

  /** HTML body with Liquid merge tags */
  body?: string;
}

/** Request body for setting a broadcast's audience */
model SetBroadcastAudienceInput {
  /** The Segment to send to; null means all active contacts */
  segmentId: EntityId | null;
}

/** Request body for a test send */
model TestSendBroadcastInput {
  /** Address to send the rendered preview to */
  email: EmailAddress;
}

/** Delivery report of a broadcast */
model BroadcastReport {
  /** Lifecycle status */
  status: BroadcastStatus;

  /**
   * Why sending is currently held, when it is: `workspace_suspended`,
   * `unverified_domain` or `no_integration`. A hold is reversible, not a failure.
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

  /** Unique opens ÷ messages sent (0-1) */
  openRate: float32;

  /** Unique clicks ÷ messages sent (0-1) */
  clickRate: float32;
}

/** Request body for scheduling a broadcast */
model ScheduleBroadcastInput {
  /** When the broadcast should be sent */
  scheduledAt: Timestamp;
}

@tag("Broadcasts")
interface Broadcasts
  extends ExternalCrud<
      BroadcastResource,
      CreateBroadcastInput,
      UpdateBroadcastInput
    > {
  /** Set the audience of a draft broadcast: a Segment, or null for all active contacts. Does not send. */
  @action("audience")
  @put
  @autoRoute
  setAudience(
    ...ResourceParameters<BroadcastResource>,
    @body body: SetBroadcastAudienceInput,
  ):
    | BroadcastResource
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Send a rendered preview with sample merge data to a single address. Never sends to the audience. */
  @action("test-send")
  @autoRoute
  testSend(
    ...ResourceParameters<BroadcastResource>,
    @body body: TestSendBroadcastInput,
  ):
    | NoContentResponse
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Schedule a draft or scheduled broadcast to send to its audience at a future time. Send-class: needs the broadcasts:send scope, and mcp:send over MCP. */
  @action("schedule")
  @autoRoute
  schedule(
    ...ResourceParameters<BroadcastResource>,
    @body body: ScheduleBroadcastInput,
  ):
    | BroadcastResource
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Cancel a scheduled broadcast and return it to draft. Send-class: needs the broadcasts:send scope, and mcp:send over MCP. */
  @action("unschedule")
  @autoRoute
  unschedule(...ResourceParameters<BroadcastResource>):
    | BroadcastResource
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Read the delivery report: sent, skipped, failed, opened and clicked counts */
  @action("report")
  @get
  @autoRoute
  report(...ResourceParameters<BroadcastResource>):
    | BroadcastReport
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem;
}
