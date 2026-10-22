using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

namespace Sphericon;

/** Rule lifecycle status */
enum RuleStatus {
  draft,
  active,
}

/** Kind of Rule action */
enum RuleActionType {
  apply_tag,
  remove_tag,
}

/** One Action of a Rule, run in order when the Rule fires. A flat shape: tag actions carry the tag name. */
model RuleAction {
  /** Action kind */
  type: RuleActionType;

  /** Tag name to apply to or remove from the Contact (apply_tag and remove_tag) */
  tag?: string;
}

/** How one Action of a Firing ended */
enum RuleActionStatus {
  succeeded,
  failed,
  skipped,
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
 * Contact and its condition holds, its Actions run at once, in order. Created Rules are drafts;
 * activating is a separate, send-class operation. */
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

  /** The Event actions that make the Rule fire; any one of them does */
  triggerEvents: string[];

  /** The Contact must match this condition for the Rule to fire; absent or empty means every Contact */
  condition?: ConditionGroup;

  /** The Rule's ordered Actions */
  actions: RuleAction[];

  /** When the Rule was last activated. It acts only on Events recorded after it. */
  activatedAt?: Timestamp | null;

  /** When the Rule was archived; null while it is kept */
  archivedAt?: Timestamp | null;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Request body for creating a Rule (always created as a draft) */
model CreateRuleInput {
  name: string;

  /** The Event actions that make the Rule fire; at least one */
  @minItems(1)
  triggerEvents: string[];

  condition?: ConditionGroup;

  /** The Actions to run, in order */
  actions?: RuleAction[];
}

/** Request body for updating a Rule */
model UpdateRuleInput {
  name?: string;

  /** The Event actions that make the Rule fire; at least one */
  @minItems(1)
  triggerEvents?: string[];

  condition?: ConditionGroup;

  /** The Actions to run, in order */
  actions?: RuleAction[];
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
      never,
      InUseProblem,
      ArchivedFilter
    > {
  /** Archive a Rule: it leaves the list and acts on nothing (an active one is deactivated), and keeps its Firings. It can be restored, as a draft. */
  @action("archive")
  @autoRoute
  archive(...ResourceParameters<RuleResource>):
    | RuleResource
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  /** Restore an archived Rule to the list, as a draft */
  @action("unarchive")
  @autoRoute
  unarchive(...ResourceParameters<RuleResource>):
    | RuleResource
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  /** Activate a Rule: it reacts to Events recorded from now on, never to past ones. Needs the automations:activate scope. */
  @action("activate")
  @autoRoute
  activate(...ResourceParameters<RuleResource>):
    | RuleResource
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Deactivate a Rule: it stops firing. Needs the automations:activate scope. */
  @action("deactivate")
  @autoRoute
  deactivate(...ResourceParameters<RuleResource>):
    | RuleResource
    | BadRequestProblem
    | UnauthorizedProblem
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
  op list(@path ruleId: EntityId, ...PageQuery):
    | Page<RuleFiringResource>
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem;
}
