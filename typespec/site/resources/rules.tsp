using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** Rule resource used by the site UI: a stateless reaction to an Event. Its definition is versioned (ADR 0065): an edit writes the draft, and Publish makes it live. Created Rules are inactive. */
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

@tag("Site Rules")
interface SiteRules
  extends WorkspaceCrud<
      SiteRuleResource,
      CreateRuleInput,
      UpdateRuleInput,
      #[],
      #[],
      #[]
    > {
  /** Publish a Rule: its draft becomes live, or with no draft its live version is resumed, and an inactive Rule becomes active and reacts to Events recorded from now on, never to past ones. 422 with neither draft nor live version. */
  @action("publish")
  @autoRoute
  @requiresPermissions(#[])
  publish(...ResourceParameters<SiteRuleResource>, @body body: RevisionInput):
    | SiteRuleResource
    | Problem;

  /** Drop the draft: the live version is again the latest. 422 when there is no draft or nothing is live. */
  @action("revert")
  @autoRoute
  @requiresPermissions(#[])
  revert(...ResourceParameters<SiteRuleResource>, @body body: RevisionInput):
    | SiteRuleResource
    | Problem;

  /** Dry-run a Rule on a stored Event: whether it would fire and which Actions would run. Writes and runs nothing. */
  @action("dry-run")
  @autoRoute
  @requiresPermissions(#[])
  dryRun(...ResourceParameters<SiteRuleResource>, @body body: DryRunRuleInput):
    | RuleDryRunResource
    | Problem;

  /** Deactivate a Rule: it stops firing */
  @action("deactivate")
  @autoRoute
  @requiresPermissions(#[])
  deactivate(...ResourceParameters<SiteRuleResource>):
    | SiteRuleResource
    | Problem;
}

@route("/workspaces/{workspaceId}/rules/{ruleId}/firings")
@tag("Site Rules")
namespace SiteRuleFirings {
  /** List a Rule's Firings, newest first */
  @get
  @requiresPermissions(#[])
  op list(
    @path workspaceId: WorkspaceId,
    @path ruleId: EntityId,
    ...PageQuery,
  ): Page<RuleFiringResource> | Problem;
}
