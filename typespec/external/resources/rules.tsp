using TypeSpec.Http;
using TypeSpec.OpenAPI;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

namespace Sphericon;

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

  /** When the Rule was deleted (discarded): it is kept with its Firings, read by id only, never fires and changes no more. Null while it is kept. */
  discardedAt?: Timestamp | null;

  /** The revision of the state this answer shows (ADR 0119): an update, Publish and Revert name it */
  revision: Revision;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

@tag("Rules")
interface Rules
  extends ExternalCrud<
      RuleResource,
      CreateRuleInput,
      UpdateRuleInput,
      ApiTokenScope.automationsRead,
      ApiTokenScope.automationsWrite,
      ApiTokenScope.automationsWrite
    > {
  /** Publish a Rule: its draft becomes live, or with no draft its live version is resumed, and an inactive Rule becomes active and reacts to Events recorded from now on, never to past ones. 422 with neither draft nor live version. Needs the automations:publish scope. */
  @action("publish")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsPublish])
  publish(...ResourceParameters<RuleResource>, @body body: RevisionInput):
    | RuleResource
    | Problem;

  /** Drop the draft: the live version is again the latest. 422 when there is no draft or nothing is live. Needs the automations:write scope. */
  @action("revert")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsWrite])
  revert(...ResourceParameters<RuleResource>, @body body: RevisionInput):
    | RuleResource
    | Problem;

  /** Dry-run a Rule on a stored Event: whether it would fire and which Actions would run. Writes and runs nothing. Needs the automations:read scope. */
  @action("dry-run")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsRead])
  dryRun(...ResourceParameters<RuleResource>, @body body: DryRunRuleInput):
    | RuleDryRunResource
    | Problem;

  /** Deactivate a Rule: it stops firing. Needs the automations:write scope. */
  @action("deactivate")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.automationsWrite])
  deactivate(...ResourceParameters<RuleResource>): RuleResource | Problem;
}

@route("/rules/{ruleId}/firings")
@tag("Rules")
namespace RuleFirings {
  /** List a Rule's Firings, newest first. Needs the automations:read scope. */
  @get
  @requiresScopes(#[ApiTokenScope.automationsRead])
  op list(@path ruleId: EntityId, ...PageQuery):
    | Page<RuleFiringResource>
    | Problem;
}
