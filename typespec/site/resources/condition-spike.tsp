using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

// SPIKE (#1674): scratch operations that store a Condition of each place and return it read back.
@route("/workspaces/{workspaceId}/condition-spike")
@tag("Site Condition Spike")
namespace SiteConditionSpike {
  /** Store a Segment's Condition and return it as read back */
  @route("/segment")
  @post
  @requiresPermissions(#[])
  op segment(
    @path workspaceId: WorkspaceId,
    @body body: SpikeSegmentConditionInput,
  ): SpikeSegmentConditionResult | Problem;

  /** Store a Rule's Condition and return it as read back */
  @route("/rule")
  @post
  @requiresPermissions(#[])
  op rule(@path workspaceId: WorkspaceId, @body body: SpikeRuleConditionInput):
    | SpikeRuleConditionResult
    | Problem;
}
