using TypeSpec.Http;
using TypeSpec.OpenAPI;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

namespace Sphericon;

/** Rule lifecycle status: active reacts to Events, inactive acts on nothing */
enum RuleStatus {
  active,
  inactive,
}

/** Kind of Rule action */
enum RuleActionType {
  apply_tag,
  remove_tag,
  enroll_automation,
  http,
}

/** One Action of a Rule, run in order when the Rule fires. A flat shape: tag actions carry the tag name, http actions the call. */
model RuleAction {
  /** Action kind */
  type: RuleActionType;

  /** Tag name to apply to or remove from the Contact (apply_tag and remove_tag) */
  tag?: string;

  /** Automation to enroll the Contact in, once ever (enroll_automation) */
  automationId?: EntityId;

  /** Action id (http actions). Assigned on save; send it back on edit so the action keeps its secrets. */
  id?: string;

  /** URL to POST to (http actions). A secret: never returned. Omit it when editing to keep the stored URL. Up to 2 KiB, http or https. */
  url?: string;

  /** Up to 5 request headers (http actions). On read, only the names are returned. */
  headers?: AutomationHttpHeader[];

  /** Liquid JSON request body (http actions): Contact bindings plus `event.action` and `event.properties`, strings JSON-escaped, up to 64 KiB, must render to valid JSON */
  body?: string;

  /** Signing secret (http actions): when set, the request is signed with Standard Webhooks. Never returned. Omit it when editing to keep the stored secret. */
  secret?: string;

  /** Remove the signing secret on save (http actions) */
  clearSecret?: boolean;

  /** Whether a signing secret is stored (http actions, read only) */
  hasSecret?: boolean;
}

/** One version of a Rule's definition (ADR 0065): its Trigger events, Condition and Actions */
model RuleVersion {
  /** 1 for the first version, plus one per version after it */
  number: int32;

  /** The Event actions that make the Rule fire; any one of them does */
  triggerEvents: string[];

  /** The Contact must match this condition for the Rule to fire; absent means every Contact */
  condition?: ConditionGroup;

  /** The Rule's ordered Actions */
  actions: RuleAction[];
}

/** How one Action of a Firing ended */
enum RuleActionStatus {
  succeeded,
  failed,
  skipped,
  pending,
}

/** What one Action did when a Rule fired */
model RuleActionResult {
  /** The Action's kind */
  type: RuleActionType;

  /** How it ended; a skipped Action is an expected outcome, not an error */
  status: RuleActionStatus;

  /** Why it failed or was skipped */
  detail?: string;
}

/** A Rule: a stateless reaction to an Event. When any of its Trigger events is recorded for a
 * Contact and its condition holds, its Actions run at once, in order. Its definition is versioned
 * (ADR 0065): an edit writes the draft, and publishing, a separate send-class operation, makes it
 * live. Created Rules are inactive. */
@resource("rules")
@segment("rules")
model RuleResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** Rule name */
  name: string;

  /** Lifecycle status */
  status: RuleStatus;

  /** What the Rule does now; null until it is first published */
  live: RuleVersion | null;

  /** Unpublished changes, the one version above live; null when there are none */
  draft: RuleVersion | null;

  /** When the Rule last became active by Publish. It acts only on Events recorded after it. */
  activatedAt?: Timestamp | null;

  /** When the Rule was archived; null while it is kept */
  archivedAt?: Timestamp | null;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Request body for creating a Rule: it is created inactive, with its definition as the draft */
model CreateRuleInput {
  name: string;

  /** The Event actions that make the Rule fire; at least one */
  @minItems(1)
  triggerEvents: string[];

  condition?: ConditionGroup;

  /** The Actions to run, in order */
  actions?: RuleAction[];
}

/** Request body for updating a Rule. The name is the Rule's own; the Trigger events, condition and Actions write the draft, and the ones left out keep the draft's, else the live version's. */
model UpdateRuleInput {
  name?: string;

  /** The Event actions that make the Rule fire; at least one */
  @minItems(1)
  triggerEvents?: string[];

  condition?: ConditionGroup;

  /** The Actions to run, in order */
  actions?: RuleAction[];
}

/** Request body for dry-running a Rule on a stored Event */
model DryRunRuleInput {
  /** The stored Event to try the Rule on */
  eventId: EntityId;
}

/** Whether a stored Event would fire a Rule, and if not, why */
enum RuleDryRunOutcome {
  would_fire,
  not_triggered,
  no_contact,
  machine,
  backfill,
  condition_not_met,
}

/** One Action a Rule would run, described without running it */
model RuleDryRunAction {
  /** The Action's kind */
  type: string;

  /** One line saying what the Action would do */
  summary: string;
}

/** What a stored Event would do to a Rule. Nothing was written or run. */
model RuleDryRunResource {
  /** Whether the Rule would fire and, if not, why */
  outcome: RuleDryRunOutcome;

  /** The Actions that would run, in order; empty unless the Rule would fire */
  actions: RuleDryRunAction[];
}

/**
 * Delete removes a Rule that has no Firings for good; one with Firings is history and is
 * archived instead (ADR 0049), so a delete of it is a 409 whose `usedBy` carries their count.
 */
@tag("Rules")
interface Rules
  extends ExternalCrud<
      RuleResource,
      CreateRuleInput,
      UpdateRuleInput,
      ApiTokenScope.automationsRead,
      ApiTokenScope.automationsWrite,
      ApiTokenScope.automationsWrite,
      never,
      InUseProblem,
      ArchivedFilter
    > {
  /** Archive a Rule: it leaves the list and acts on nothing (an active one is deactivated), and keeps its Firings. It can be restored, inactive. */
  @action("archive")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsWrite])
  archive(...ResourceParameters<RuleResource>):
    | RuleResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  /** Restore an archived Rule to the list, inactive */
  @action("unarchive")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsWrite])
  unarchive(...ResourceParameters<RuleResource>):
    | RuleResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  /** Publish a Rule: its draft becomes live, or with no draft its live version is resumed, and an inactive Rule becomes active and reacts to Events recorded from now on, never to past ones. 422 with neither draft nor live version. Needs the automations:publish scope. */
  @action("publish")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsPublish])
  publish(...ResourceParameters<RuleResource>):
    | RuleResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Drop the draft: the live version is again the latest. 422 when there is no draft or nothing is live. Needs the automations:write scope. */
  @action("revert")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsWrite])
  revert(...ResourceParameters<RuleResource>):
    | RuleResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Dry-run a Rule on a stored Event: whether it would fire and which Actions would run. Writes and runs nothing. Needs the automations:read scope. */
  @action("dry-run")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsRead])
  dryRun(...ResourceParameters<RuleResource>, @body body: DryRunRuleInput):
    | RuleDryRunResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  /** Deactivate a Rule: it stops firing. Needs the automations:write scope. */
  @action("deactivate")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsWrite])
  deactivate(...ResourceParameters<RuleResource>):
    | RuleResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;
}

/** One execution of a Rule for one Event, with what each Action did. Erased with the Contact. */
model RuleFiringResource {
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
  results: RuleActionResult[];

  /** When the last Action finished; absent while Actions are still to run */
  completedAt?: Timestamp;

  /** When the Rule fired */
  createdAt: Timestamp;
}

@route("/rules/{ruleId}/firings")
@tag("Rules")
namespace RuleFirings {
  /** List a Rule's Firings, newest first. Needs the automations:read scope. */
  @get
  @requiresScopes(#[ApiTokenScope.automationsRead])
  op list(@path ruleId: EntityId, ...PageQuery):
    | Page<RuleFiringResource>
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;
}
