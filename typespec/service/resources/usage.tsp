import "../../common/pagination.tsp";

using TypeSpec.Http;

namespace Sphericon;

/** A metric of Usage */
enum UsageMetric {
  /** Emails sent: one per recipient across Broadcast, Automation and Transactional */
  emailsSent: "emails_sent",

  /** Contacts: the peak number of identified Contacts (those with an email Destination) reached
   * during the Usage period */
  contacts: "contacts",
}

/** One metric's figure for a Workspace over a Usage period */
model UsageFigure {
  metric: UsageMetric;

  /** The value of the metric over the period */
  value: int64;

  /** True once the Usage period closed: the figure is a Usage snapshot and never changes.
   * False while it is Live usage. */
  final: boolean;

  /** When the figure was last computed */
  computedAt: Timestamp;
}

/** The figures of one Workspace for one Usage period */
model WorkspaceUsageResource {
  /** The Workspace */
  workspaceId: EntityId;

  /** The Usage period: the calendar month in UTC, as YYYY-MM */
  period: string;

  /** One figure per metric */
  figures: UsageFigure[];
}

/** Which Usage period to read */
model UsagePeriodQuery {
  /** The Usage period as YYYY-MM (UTC); the current one when absent */
  @query
  @pattern("^[0-9]{4}-(0[1-9]|1[0-2])$")
  period?: string;
}

@tag("Service Usage")
@route("/usage")
namespace ServiceUsage {
  /** The figures of every Workspace for a Usage period, by Workspace id: Live usage for
   * the open period, Usage snapshots for a closed one. Never a recount on read. */
  @get
  op list(...UsagePeriodQuery, ...PageQuery):
    | Page<WorkspaceUsageResource>
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem;
}

@tag("Service Usage")
@route("/workspaces/{workspaceId}/usage")
namespace ServiceWorkspaceUsage {
  /** The figures of one Workspace for a Usage period: Live usage (not final, counting
   * sends still in flight) for the open period, Usage snapshots (final) for a closed one.
   * Never a recount on read. */
  @get
  op get(@path workspaceId: EntityId, ...UsagePeriodQuery):
    | WorkspaceUsageResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | NotFoundProblem
    | TooManyRequestsProblem;
}
