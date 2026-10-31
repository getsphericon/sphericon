using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** Broadcast lifecycle status */
enum SiteBroadcastStatus {
  draft,
  scheduled,
  sending,
  sent,
  failed,
}

/** The Orders credited to one Broadcast or Automation and the net revenue of their Movements
 * (ADR 0064), in the Reporting currency, read from the Movements and never stored: a refund
 * lowers `amount` on its own day. Until the currency is chosen `reportingCurrency` is null and
 * there is no figure to show. Orders in another currency are counted in neither field. */
model SiteMessageRevenue {
  /** The Reporting currency, null until chosen */
  reportingCurrency: CurrencyCode | null;

  /** Orders completed that are credited to it, in the Reporting currency */
  orders: int32;

  /** Net revenue of those Orders, in the minor unit of the Reporting currency */
  amount: int64;
}

/** Denormalized delivery counters for a broadcast */
model SiteBroadcastStats {
  /** Number of recipients the broadcast targets */
  recipientsTotal: int32;

  /** Messages successfully handed to the provider */
  sentCount: int32;

  /** Recipients that opened the email */
  openedCount: int32;

  /** Recipients that clicked a link */
  clickedCount: int32;

  /** Recipients that unsubscribed via this broadcast */
  unsubscribedCount: int32;

  /** Messages that failed to send */
  failedCount: int32;

  /** Messages the provider reported delivered, read from the delivery result stamped on each message */
  deliveredCount: int32;

  /** Messages the provider reported bounced, read from the delivery result stamped on each message */
  bouncedCount: int32;

  /** Messages a recipient complained about, read from the delivery result stamped on each message */
  complainedCount: int32;

  /** Messages that went through a Test inbox: captured, not delivered; they count as delivered too */
  capturedCount: int32;

  /** Messages handed to the provider ÷ recipients targeted (0–1) */
  deliveryRate: float32;

  /** Unique opens ÷ messages sent (0–1) */
  openRate: float32;

  /** Unique clicks ÷ messages sent (0–1) */
  clickRate: float32;

  /** Unique clicks ÷ unique opens (0–1) */
  clickToOpenRate: float32;

  /** Unsubscribes ÷ messages sent (0–1) */
  unsubscribeRate: float32;

  /** Send failures ÷ recipients targeted (0–1) */
  failureRate: float32;

  /** Orders credited to the Broadcast and their net revenue (ADR 0064) */
  revenue: SiteMessageRevenue;
}

/**
 * How far a sending broadcast has got. A broadcast limited by its Integration's Send
 * rate limit is paced: recipients go out at an even interval, so the estimate is
 * derived on the server from the last scheduled time and the recipients remaining.
 */
model SiteBroadcastProgress {
  /** Recipients already decided: sent, skipped or failed */
  processedCount: int32;

  /** Recipients still waiting to be sent */
  remainingCount: int32;

  /**
   * When the last remaining recipient is expected to be sent. Null when it is not
   * knowable: the broadcast is on hold, or it is not paced by a Provider limit.
   */
  estimatedCompletionAt?: Timestamp | null;
}

/** Broadcast resource used by the site UI */
@resource("broadcasts")
@segment("broadcasts")
@parentResource(SiteWorkspaceResource)
model SiteBroadcastResource {
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

  /** Warnings of the lint over the stored content (ADR 0040); set on the response of a save, never blocks it */
  warnings?: LintFinding[];

  /**
   * Target segment; null means all contacts. Once the broadcast leaves draft it holds a copy of
   * the segment's condition taken then, and this names the segment only as where its audience
   * came from.
   */
  segmentId?: EntityId | null;

  /** Sending integration; null means the workspace default */
  integrationId?: EntityId | null;

  /** Lifecycle status */
  status: SiteBroadcastStatus;

  /**
   * Why sending is currently held, when it is: `workspace_suspended`,
   * `unverified_domain` or `no_integration`. A hold is reversible and not a failure:
   * the broadcast keeps its status and the remaining recipients send once it lifts.
   */
  holdReason?: string | null;

  /** When the broadcast is scheduled to send */
  scheduledAt?: Timestamp | null;

  /** When the broadcast finished sending */
  sentAt?: Timestamp | null;

  /** Delivery counters */
  stats: SiteBroadcastStats;

  /** Progress and ETA while the broadcast is sending; null otherwise. Returned by get and send only, never by the list */
  progress?: SiteBroadcastProgress | null;

  /** When the broadcast was archived; null while it is kept. Only a sent or failed broadcast is archived. */
  archivedAt?: Timestamp | null;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** What a test send did: the address the preview went out from */
model SiteTestSendBroadcastResult {
  /**
   * The From address of the preview. A Workspace with a verified Sending domain gets its own
   * domain; one without gets the System sender, which is not the address Contacts will see.
   */
  from: EmailAddress;

  /**
   * true when the Workspace's default Integration keeps what it is sent (a Test inbox): the
   * preview was captured there, not sent to the address named.
   */
  captured: boolean;
}

/** Filter of the broadcast list */
model SiteBroadcastsListFilter {
  /** true lists the archived broadcasts instead of the kept ones; the default hides them */
  @query archived?: boolean;
}

/**
 * Delete removes a draft for good; a sent or failed broadcast is history and is archived
 * instead (ADR 0049), so a delete of any other status is a 422.
 */
@tag("Site Broadcasts")
interface SiteBroadcasts
  extends WorkspaceCrud<
      SiteBroadcastResource,
      CreateBroadcastInput,
      UpdateBroadcastInput,
      never,
      UnprocessableEntityProblem,
      SiteBroadcastsListFilter
    > {
  /**
   * Archive a sent or failed broadcast: it leaves the list, keeps its report, tracking and
   * figures, and can be restored. A draft is deleted and a scheduled or sending broadcast
   * cannot be archived (422).
   */
  @action("archive")
  @autoRoute
  @extension("x-required-permissions", #[])
  archive(...ResourceParameters<SiteBroadcastResource>):
    | SiteBroadcastResource
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Restore an archived broadcast to the list */
  @action("unarchive")
  @autoRoute
  @extension("x-required-permissions", #[])
  unarchive(...ResourceParameters<SiteBroadcastResource>):
    | SiteBroadcastResource
    | BadRequestProblem
    | NotFoundProblem;

  /** Send a broadcast immediately. A draft copies its segment's condition, which its recipients are resolved from; an archived segment is refused. */
  @action("send")
  @autoRoute
  @extension("x-required-permissions", #[])
  send(...ResourceParameters<SiteBroadcastResource>):
    | SiteBroadcastResource
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Schedule a broadcast to send at a future time. A draft copies its segment's condition: recipients are resolved at send time from that copy, so a later edit of the segment changes nothing for it, and rescheduling keeps it. An archived segment is refused. */
  @action("schedule")
  @autoRoute
  @extension("x-required-permissions", #[])
  schedule(
    ...ResourceParameters<SiteBroadcastResource>,
    @body body: ScheduleBroadcastInput,
  ):
    | SiteBroadcastResource
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /**
   * Send a one-off rendered preview to a single address. While the Workspace has no verified
   * Sending domain it goes from the System sender and only to the asking User's own address
   * (ADR 0041): another address is refused with 422, the System sender's caps answer 429.
   */
  @action("test-send")
  @autoRoute
  @extension("x-required-permissions", #[])
  testSend(
    ...ResourceParameters<SiteBroadcastResource>,
    @body body: TestSendBroadcastInput,
  ):
    | SiteTestSendBroadcastResult
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem
    | TooManyRequestsProblem;
}

/** A broadcast's per-link click figures, most clicked first */
model SiteBroadcastLinkClicksList {
  /** One entry per clicked link */
  items: SiteLinkClicks[];
}

@route("/workspaces/{workspaceId}/broadcasts/{broadcastId}/links")
@tag("Site Broadcasts")
namespace SiteBroadcastLinks {
  /**
   * The links of a broadcast that were clicked, with a count each, most clicked first. The counts
   * come from the `email.clicked` Events, so they cover the Event retention window; the unique
   * click figure in the broadcast's stats does not.
   */
  @get
  @extension("x-required-permissions", #[])
  op list(@path workspaceId: WorkspaceId, @path broadcastId: EntityId):
    | SiteBroadcastLinkClicksList
    | BadRequestProblem
    | NotFoundProblem;
}

@@extension(SiteBroadcasts.create, "x-required-permissions", #[]);
@@extension(SiteBroadcasts.delete, "x-required-permissions", #[]);
@@extension(SiteBroadcasts.get, "x-required-permissions", #[]);
@@extension(SiteBroadcasts.list, "x-required-permissions", #[]);
@@extension(SiteBroadcasts.update, "x-required-permissions", #[]);
