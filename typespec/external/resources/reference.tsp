import "../../common/rates.tsp";

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** Custom field definition: a typed, named Contact attribute (ADR 0006) */
model CustomFieldResource {
  /** Unique identifier */
  id: EntityId;

  /** Machine key, how values are addressed in custom fields and segment conditions */
  @untrusted
  key: string;

  /** Display name */
  @untrusted
  name: string;

  /** Value type: inferred from the first value, changed by an owner or admin */
  type: CustomFieldType;

  /** The operators a rule leaf on this field takes, by its type */
  ruleOperators: ConditionOperator[];

  /** The revision of the state this answer shows (ADR 0119): an update names it */
  revision: Revision;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/**
 * Complaint and bounce rates of one Sending domain over a trailing window (ADR 0011).
 * Complaint rate is complaints / delivered (ADR 0053), undefined with no delivery notice in the
 * window; bounce rate is hard bounces / sent.
 */
model SendingDomainRatesResource {
  /** Sending domain id */
  sendingDomainId: EntityId;

  /** The domain the rates are for */
  domain: string;

  /** Trailing window the rates cover, in days */
  windowDays: int32;

  /** Complaints over delivered mail; the rate is null when nothing was delivered in the window */
  complaintRate: RateTriple;

  /** Hard bounces over sent */
  bounceRate: RateTriple;
}

@route("/custom-fields")
@tag("Custom fields")
namespace CustomFields {
  /** List the workspace's custom field definitions */
  @get
  @requiresScopes(#[ApiTokenScope.customFieldsRead])
  op list(): Page<CustomFieldResource> | Problem;

  /** Rename a custom field or change its type. Refused with 409 while a segment, a rule or a scheduled broadcast tests the field with an operator the new type does not take (`usedBy` names them). */
  @route("/{id}")
  @put
  @requiresScopes(#[ApiTokenScope.customFieldsWrite])
  op update(@path id: EntityId, @body body: UpdateCustomFieldInput):
    | CustomFieldResource
    | Problem;
}

@route("/sending-domains/rates")
@tag("Sending domains")
namespace SendingDomainRates {
  /** Complaint and bounce rates per sending domain over a trailing window */
  @get
  @requiresScopes(#[ApiTokenScope.sendingDomainsRead])
  op list(
    /** Trailing window in days (1 to 90) */
    @query
    @minValue(1)
    @maxValue(90)
    windowDays?: int32 = 7,
  ): Page<SendingDomainRatesResource> | Problem;
}
