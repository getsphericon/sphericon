using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

namespace Sphericon;

/** Rule lifecycle status */
enum SiteRuleStatus {
  draft,
  active,
}

/** Kind of Rule action */
enum SiteRuleActionType {
  apply_tag,
  remove_tag,
  enroll_automation,
  http,
}

/** One Action of a Rule, run in order when the Rule fires. A flat shape: tag actions carry the tag name, http actions the call. */
model SiteRuleAction {
  /** Action kind */
  type: SiteRuleActionType;

  /** Tag name to apply to or remove from the Contact (apply_tag and remove_tag) */
  tag?: string;

  /** Automation to enroll the Contact in, once ever (enroll_automation) */
  automationId?: EntityId;

  /** Action id (http actions). Assigned on save; send it back on edit so the action keeps its secrets. */
  id?: string;

  /** URL to POST to (http actions). A secret: never returned. Omit it when editing to keep the stored URL. Up to 2 KiB, http or https. */
  url?: string;

  /** Up to 5 request headers (http actions). On read, only the names are returned. */
  headers?: SiteAutomationHttpHeader[];

  /** Liquid JSON request body (http actions): Contact bindings plus `event.action` and `event.properties`, strings JSON-escaped, up to 64 KiB, must render to valid JSON */
  body?: string;

  /** Signing secret (http actions): when set, the request is signed with Standard Webhooks. Never returned. Omit it when editing to keep the stored secret. */
  secret?: string;

  /** Remove the signing secret on save (http actions) */
  clearSecret?: boolean;

  /** Whether a signing secret is stored (http actions, read only) */
  hasSecret?: boolean;
}

/** How one Action of a Firing ended */
enum SiteRuleActionStatus {
  succeeded,
  failed,
  skipped,
  pending,
}

/** What one Action did when a Rule fired */
model SiteRuleActionResult {
  /** The Action's kind */
  type: SiteRuleActionType;

  /** How it ended; a skipped Action is an expected outcome, not an error */
  status: SiteRuleActionStatus;

  /** Why it failed or was skipped */
  detail?: string;
}

/** Rule resource used by the site UI: a stateless reaction to an Event. Created Rules are drafts. */
@resource("rules")
@segment("rules")
@parentResource(SiteWorkspaceResource)
model SiteRuleResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** Rule name */
  name: string;

  /** Lifecycle status */
  status: SiteRuleStatus;

  /** The Event actions that make the Rule fire; any one of them does */
  triggerEvents: string[];

  /** The Contact must match this condition for the Rule to fire; absent or empty means every Contact */
  condition?: SiteConditionGroup;

  /** The Rule's ordered Actions */
  actions: SiteRuleAction[];

  /** When the Rule was last activated. It acts only on Events recorded after it. */
  activatedAt?: Timestamp | null;

  /** When the Rule was archived; null while it is kept */
  archivedAt?: Timestamp | null;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Site request body for creating a Rule (always created as a draft) */
model SiteCreateRuleInput {
  name: string;

  /** The Event actions that make the Rule fire; at least one */
  @minItems(1)
  triggerEvents: string[];

  condition?: SiteConditionGroup;

  /** The Actions to run, in order */
  actions?: SiteRuleAction[];
}

/** Site request body for updating a Rule */
model SiteUpdateRuleInput {
  name?: string;

  /** The Event actions that make the Rule fire; at least one */
  @minItems(1)
  triggerEvents?: string[];

  condition?: SiteConditionGroup;

  /** The Actions to run, in order */
  actions?: SiteRuleAction[];
}

/** Site request body for dry-running a Rule on a stored Event */
model SiteDryRunRuleInput {
  /** The stored Event to try the Rule on */
  eventId: EntityId;
}

/** Whether a stored Event would fire a Rule, and if not, why */
enum SiteRuleDryRunOutcome {
  would_fire,
  not_triggered,
  no_contact,
  machine,
  backfill,
  condition_not_met,
}

/** One Action a Rule would run, described without running it */
model SiteRuleDryRunAction {
  /** The Action's kind */
  type: string;

  /** One line saying what the Action would do */
  summary: string;
}

/** What a stored Event would do to a Rule. Nothing was written or run. */
model SiteRuleDryRunResource {
  /** Whether the Rule would fire and, if not, why */
  outcome: SiteRuleDryRunOutcome;

  /** The Actions that would run, in order; empty unless the Rule would fire */
  actions: SiteRuleDryRunAction[];
}

@tag("Site Rules")
interface SiteRules
  extends WorkspaceCrud<
      SiteRuleResource,
      SiteCreateRuleInput,
      SiteUpdateRuleInput,
      never,
      InUseProblem,
      ArchivedFilter
    > {
  /**
   * Archive a Rule: it leaves the list and acts on nothing (an active one is deactivated), and
   * keeps its Firings. It can be restored, as a draft.
   */
  @action("archive")
  @autoRoute
  archive(...ResourceParameters<SiteRuleResource>):
    | SiteRuleResource
    | BadRequestProblem
    | NotFoundProblem;

  /** Restore an archived Rule to the list, as a draft */
  @action("unarchive")
  @autoRoute
  unarchive(...ResourceParameters<SiteRuleResource>):
    | SiteRuleResource
    | BadRequestProblem
    | NotFoundProblem;

  /** Activate a Rule: it reacts to Events recorded from now on, never to past ones */
  @action("activate")
  @autoRoute
  activate(...ResourceParameters<SiteRuleResource>):
    | SiteRuleResource
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Dry-run a Rule on a stored Event: whether it would fire and which Actions would run. Writes and runs nothing. */
  @action("dry-run")
  @autoRoute
  dryRun(
    ...ResourceParameters<SiteRuleResource>,
    @body body: SiteDryRunRuleInput,
  ): SiteRuleDryRunResource | BadRequestProblem | NotFoundProblem;

  /** Deactivate a Rule: it stops firing */
  @action("deactivate")
  @autoRoute
  deactivate(...ResourceParameters<SiteRuleResource>):
    | SiteRuleResource
    | BadRequestProblem
    | NotFoundProblem;
}

/** One execution of a Rule for one Event, with what each Action did. Erased with the Contact. */
model SiteRuleFiringResource {
  /** Unique identifier */
  id: EntityId;

  /** The Rule that fired */
  ruleId: EntityId;

  /** The Contact the Event was for */
  contactId: EntityId;

  /** The Trigger event: the action of the Event that fired the Rule */
  action: string;

  /** The Event's source id: the same Event delivered twice fires a Rule once */
  eventSourceId: string;

  /** One result per Action run so far, in the Rule's order */
  results: SiteRuleActionResult[];

  /** When the last Action finished; absent while Actions are still to run */
  completedAt?: Timestamp;

  /** When the Rule fired */
  createdAt: Timestamp;
}

@route("/workspaces/{workspaceId}/rules/{ruleId}/firings")
@tag("Site Rules")
namespace SiteRuleFirings {
  /** List a Rule's Firings, newest first */
  @get
  op list(
    @path workspaceId: WorkspaceId,
    @path ruleId: EntityId,
    ...PageQuery,
  ): Page<SiteRuleFiringResource> | BadRequestProblem | NotFoundProblem;
}
