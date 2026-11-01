using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** Selectable window for the analytics overview */
enum SiteAnalyticsRange {
  `7d`,
  `30d`,
  `90d`,
}

/** Current contact counts (point-in-time snapshot, not range-scoped) */
model SiteAnalyticsContacts {
  /** All contacts in the workspace */
  total: int32;

  /** Contacts with status = active */
  active: int32;

  /** Contacts with status = unsubscribed */
  unsubscribed: int32;

  /** Contacts created within the selected range */
  newInRange: int32;
}

/**
 * The messages sent in the range through every surface, read from the Outbound messages
 * themselves (ADR 0053): what went out and what the provider said became of it. A Workspace that
 * only runs Automations or Transactional sends has its sends here, not in `email`.
 */
model SiteAnalyticsSending {
  /** Messages of Broadcasts sent in the range */
  broadcast: int32;

  /** Messages of Automations sent in the range */
  automation: int32;

  /** Transactional messages sent in the range */
  transactional: int32;

  /** Of all the messages sent in the range, those the provider reported delivered */
  delivered: int32;

  /** Of all the messages sent in the range, those the provider reported bounced */
  bounced: int32;

  /** Of all the messages sent in the range, those a recipient marked as spam */
  complained: int32;
}

/**
 * Email engagement for the Broadcast messages sent in the selected range (the send
 * cohort), from delivery logs. Opens/clicks are counted among that cohort, so
 * the counts reconcile and the rates stay in [0,1].
 */
model SiteAnalyticsEmail {
  /** Messages sent within the range */
  sentCount: int32;

  /** Messages sent in the range that were opened */
  openedCount: int32;

  /** Messages sent in the range that were clicked */
  clickedCount: int32;

  /** Opened ÷ sent (0–1) */
  openRate: float32;

  /** Clicked ÷ sent (0–1) */
  clickRate: float32;

  /** Clicked ÷ opened (0–1) */
  clickToOpenRate: float32;
}

/** Automation counts (point-in-time snapshot) */
model SiteAnalyticsAutomations {
  /** All automations in the workspace */
  total: int32;

  /** Automations with status = active */
  active: int32;

  /** Runs currently in progress */
  runsActive: int32;

  /** Runs that have completed */
  runsCompleted: int32;
}

/**
 * One Report day of the cohort (ADR 0057), zero-filled across the range. Opened/clicked
 * count the messages sent that day that were later opened/clicked.
 */
model SiteAnalyticsPoint {
  /** The Report day, `YYYY-MM-DD` in the Workspace time zone */
  date: string;

  /** Messages sent that day */
  sent: int32;

  /** Messages sent that day that were opened */
  opened: int32;

  /** Messages sent that day that were clicked */
  clicked: int32;
}

/** The metric of a Sending domain that is over a warning limit of the abuse detector */
enum SiteAbuseMetric {
  complaint,
  bounce,
}

/** A Sending domain in the warning tier of the abuse detector (ADR 0026): its rate is
 * over the warning limit but under the freeze limit. Computed live from the trailing
 * 24 hours since any clearance, never stored. */
model SiteAbuseWarning {
  /** The Sending domain over the limit */
  domain: string;

  /** Which rate is over */
  metric: SiteAbuseMetric;

  /** The rate, 0–1 */
  rate: float64;
}

/** Workspace analytics overview powering the dashboard */
model SiteAnalyticsOverview {
  contacts: SiteAnalyticsContacts;
  sending: SiteAnalyticsSending;
  email: SiteAnalyticsEmail;
  automations: SiteAnalyticsAutomations;

  /** Daily engagement series, oldest first, one point per day in the range */
  timeseries: SiteAnalyticsPoint[];
}

/**
 * One Report day of the New people report: the Contacts created that day by the door they
 * came through, zero-filled across the range.
 */
model SiteNewPeoplePoint {
  /** Report day, YYYY-MM-DD in the Workspace time zone */
  date: string;

  /** Contacts created by hand that day */
  manual: int32;

  /** Contacts created through the API that day */
  api: int32;

  /** Contacts created by the tracker that day */
  tracker: int32;

  /** Contacts that predate the Contact source */
  `unknown`: int32;
}

/** New people: Contacts created in the selected range, per Report day and by Contact source */
model SiteNewPeopleReport {
  /** Contacts created within the range */
  total: int32;

  /** One point per Report day in the range, oldest first */
  timeseries: SiteNewPeoplePoint[];
}

/** One Report day of the Subscribed people report, zero-filled across the range */
model SiteAnalyticsSubscribedPoint {
  /** Report day, YYYY-MM-DD in the Workspace time zone */
  date: string;

  /** People who subscribed that day (marketing.confirmed events) */
  subscribed: int32;

  /** People who unsubscribed that day (email.unsubscribed events) */
  unsubscribed: int32;

  /** Subscribed minus unsubscribed; negative when more people left than joined */
  net: int32;
}

/** The Subscribed people report: the daily flow plus the current number, which is read from
 * current state and is never reconstructed from the flow */
model SiteAnalyticsSubscribedPeople {
  /** Contacts that are not globally opted out of email now */
  current: int32;

  /** Daily flow, oldest first, one point per Report day in the range */
  series: SiteAnalyticsSubscribedPoint[];
}

/** What carried a clicked link: a Broadcast or an Automation */
enum SiteAnalyticsOwnerKind {
  broadcast,
  automation,
}

/** The clicks of one link in the emails of one Broadcast or Automation */
model SiteAnalyticsLinkOwner {
  /** Whether the Broadcast report or the Automation report is the one to open */
  kind: SiteAnalyticsOwnerKind;

  /** The Broadcast's or Automation's id */
  id: EntityId;

  /** The Broadcast's or Automation's name */
  name: string;

  /** Human clicks on the link in its emails during the range */
  clicks: int32;
}

/** One clicked link with its clicks in the range, split by the Broadcast or Automation that carried it */
model SiteAnalyticsLink {
  /** The link's destination URL */
  url: string;

  /** Human clicks on the link during the range (every click counted, not unique per contact) */
  clicks: int32;

  /** Where the clicks came from, most clicked first */
  owners: SiteAnalyticsLinkOwner[];
}

/** The Clicks by link report */
model SiteAnalyticsClicksByLink {
  /** Each link with clicks in the range, most clicked first; machine clicks are left out */
  links: SiteAnalyticsLink[];

  /** The configured Event retention in days (SPHERICON_EVENTS_RETENTION_DAYS); 0 means Events
   * are kept forever. Clicks are Events, so a range longer than this is counted only over the
   * retained days. */
  eventsRetentionDays: int32;
}

/** Human opens of one weekday and hour of day in the Workspace time zone */
model SiteAnalyticsOpensCell {
  /** ISO weekday: 1 is Monday, 7 is Sunday */
  weekday: int32;

  /** Hour of day, 0-23 */
  hour: int32;

  /** Human opens in that hour of that weekday within the range */
  opens: int32;
}

/** Opens by hour report: a cell for every weekday and hour, Monday 00:00 first; machine opens are left out */
model SiteAnalyticsOpensByHour {
  cells: SiteAnalyticsOpensCell[];

  /** Human opens in the range */
  total: int32;
}

/** One action of the Events report */
model SiteAnalyticsEventAction {
  /** The Event action, reserved `email.*` or tracked by the customer */
  action: string;

  /** How many times it happened in the range */
  count: int32;

  /** How many distinct Contacts it happened to; anonymous Events count in `count` only */
  contacts: int32;

  /** When it last happened in the range */
  lastAt: Timestamp;
}

/** The Events report: each action in the range, most frequent first. Machine opens and
 * clicks are left out. */
model SiteAnalyticsEvents {
  /** One row per action */
  actions: SiteAnalyticsEventAction[];
}

/** One Report day of the Workspace's revenue (ADR 0064): the sum of the Movements of the Orders
 * counted in the Reporting currency that fell on that day in the Workspace time zone. A day
 * with none is there with zeros. */
model SiteRevenuePoint {
  /** The Report day, `YYYY-MM-DD` in the Workspace time zone */
  date: string;

  /** The net change of revenue that day, in the minor unit of the Reporting currency (cents
   * for USD, whole yen for JPY); a refund makes it negative */
  amount: int64;

  /** Orders completed that day */
  orders: int32;
}

/** Orders completed in the range in one currency other than the Reporting currency */
model SiteOrdersInCurrency {
  /** The currency the Orders are in */
  currency: CurrencyCode;

  /** Orders completed in the range in that currency */
  orders: int32;
}

/** The revenue of the orders the Workspace sent to Sphericon (ADR 0064), not of its shop: the
 * sum of Order Movements over the range, in the Reporting currency. Until the first Order
 * Completed sets that currency, or an Owner or Admin chooses it, `reportingCurrency` is null and
 * every figure is zero. */
model SiteAnalyticsRevenue {
  /** The Reporting currency, null until the first Order or an Owner or Admin sets it */
  reportingCurrency: CurrencyCode | null;

  /** When the Workspace's first Order was completed, in any currency and whatever the range; null
   * while it has sent none. A Reporting currency chosen before any Order leaves it null. */
  firstOrderAt: Timestamp | null;

  /** Revenue over the range, in the minor unit of the Reporting currency */
  total: int64;

  /** The part of `total` credited to messages (ADR 0064): the sum of the Movements of the Orders
   * credited to a Broadcast or an Automation, in the minor unit of the Reporting currency.
   * `total` minus `credited` is the revenue of unattributed Orders. */
  credited: int64;

  /** Orders completed in the range, in the Reporting currency */
  orders: int32;

  /** Orders completed in the range that are credited to a Broadcast or an Automation */
  creditedOrders: int32;

  /** Orders completed in the range in another currency, one entry per currency ordered by its
   * code: they are listed with the Workspace's Events but counted in no sum */
  otherCurrencyOrders: SiteOrdersInCurrency[];

  /** One point per Report day in the range, oldest first, zero-filled */
  series: SiteRevenuePoint[];
}

// Not a CRUD resource — a single workspace-scoped aggregate with no key, so it
// keeps an explicit nested route rather than @parentResource/@autoRoute.
@route("/workspaces/{workspaceId}/analytics")
@tag("Site Analytics")
namespace SiteAnalytics {
  /** Aggregate metrics for the workspace dashboard */
  @get
  @route("/overview")
  @requiresPermissions(#[])
  op overview(
    @path workspaceId: WorkspaceId,

    /** Window to aggregate over (defaults to 7d) */
    @query range?: SiteAnalyticsRange,
  ): SiteAnalyticsOverview | BadRequestProblem | NotFoundProblem;

  /** Revenue of the Workspace's Orders over the range, in its Reporting currency (ADR 0064).
   * Open to every member. */
  @get
  @route("/revenue")
  @requiresPermissions(#[])
  op revenue(
    @path workspaceId: WorkspaceId,

    /** Window to sum over (defaults to 30d) */
    @query range?: SiteAnalyticsRange = SiteAnalyticsRange.`30d`,
  ): SiteAnalyticsRevenue | BadRequestProblem | NotFoundProblem;

  /** New people report: Contacts created per Report day, split by Contact source */
  @get
  @route("/new-people")
  @requiresPermissions(#[])
  op newPeople(
    @path workspaceId: WorkspaceId,

    /** Window to aggregate over (defaults to 7d) */
    @query range?: SiteAnalyticsRange,
  ): SiteNewPeopleReport | BadRequestProblem | NotFoundProblem;

  /** Subscribed people report: who subscribed and unsubscribed per Report day, and how many are subscribed now */
  @get
  @route("/subscribed-people")
  @requiresPermissions(#[])
  op subscribedPeople(
    @path workspaceId: WorkspaceId,

    /** Window to report over (defaults to 7d) */
    @query range?: SiteAnalyticsRange,
  ): SiteAnalyticsSubscribedPeople | BadRequestProblem | NotFoundProblem;

  /** Clicks by link over the whole workspace, split by the Broadcast or Automation that carried each link */
  @get
  @route("/clicks-by-link")
  @requiresPermissions(#[])
  op clicksByLink(
    @path workspaceId: WorkspaceId,

    /** Window to aggregate over (defaults to 7d) */
    @query range?: SiteAnalyticsRange,
  ): SiteAnalyticsClicksByLink | BadRequestProblem | NotFoundProblem;

  /** Human opens by hour of day and day of week in the Workspace time zone */
  @get
  @route("/opens-by-hour")
  @requiresPermissions(#[])
  op opensByHour(
    @path workspaceId: WorkspaceId,

    /** Window to aggregate over (defaults to 7d) */
    @query range?: SiteAnalyticsRange,
  ): SiteAnalyticsOpensByHour | BadRequestProblem | NotFoundProblem;

  /** Events in the range, grouped by action */
  @get
  @route("/events")
  @requiresPermissions(#[])
  op events(
    @path workspaceId: WorkspaceId,

    /** Window to report over (defaults to 7d) */
    @query range?: SiteAnalyticsRange,
  ): SiteAnalyticsEvents | BadRequestProblem | NotFoundProblem;
}
