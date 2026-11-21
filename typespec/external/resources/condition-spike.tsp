using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

// SPIKE (#1674): scratch operations that store a Condition of each place and return it read back.
@route("/condition-spike")
@tag("Condition Spike")
namespace ConditionSpike {
  /** Store a Segment's Condition and return it as read back */
  @route("/segment")
  @post
  @requiresScopes(#[ApiTokenScope.segmentsWrite])
  op segment(@body body: SpikeSegmentConditionInput):
    | SpikeSegmentConditionResult
    | Problem;

  /** Store a Rule's Condition and return it as read back */
  @route("/rule")
  @post
  @requiresScopes(#[ApiTokenScope.segmentsWrite])
  op rule(@body body: SpikeRuleConditionInput):
    | SpikeRuleConditionResult
    | Problem;
}
