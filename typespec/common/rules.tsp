// The Rule models /site and /api show and take the same way: its Actions, versions, write bodies,
// dry run and firings.

import "./segments.tsp";
import "./signing-secret.tsp";

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
model RuleActionOf<Header> {
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
  @maxItems(5)
  headers?: Header[];

  /** Liquid JSON request body (http actions): Contact bindings plus `event.action` and `event.properties`, strings JSON-escaped, up to 64 KiB, must render to valid JSON */
  body?: string;

  /** Signing secret (http actions): when set, the request is signed with Standard Webhooks. Never returned. Omit it when editing to keep the stored secret. */
  secret?: SigningSecret;

  /** Remove the signing secret on save (http actions) */
  clearSecret?: boolean;

  /** Whether a signing secret is stored (http actions, read only) */
  hasSecret?: boolean;
}

model RuleAction is RuleActionOf<AutomationHttpHeader>;

// The action a request sends is closed at every depth, the one a response returns stays open (ADR
// 0148): one shape, two models.
model RuleActionInput is RuleActionOf<AutomationHttpHeaderInput>;

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

/** Request body for creating a Rule: it is created inactive, with its definition as the draft */
model CreateRuleInput {
  name: Name;

  /** The Event actions that make the Rule fire; at least one */
  @minItems(1)
  triggerEvents: string[];

  condition?: ConditionGroupInput;

  /** The Actions to run, in order */
  actions?: RuleActionInput[];
}

/** Request body for updating a Rule. The name is the Rule's own; the Trigger events, condition and Actions write the draft, and the ones left out keep the draft's, else the live version's. */
model UpdateRuleInput {
  /** The rule's `revision` the edit was made on (ADR 0119): the one it was read with */
  revision: Revision;

  name?: Name;

  /** The Event actions that make the Rule fire; at least one */
  @minItems(1)
  triggerEvents?: string[];

  condition?: ConditionGroupInput;

  /** The Actions to run, in order */
  actions?: RuleActionInput[];
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
