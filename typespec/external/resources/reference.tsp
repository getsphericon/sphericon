using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** A Custom field's value type */
enum CustomFieldType {
  string,
  number,
  bool,
  datetime,
}

/** Custom field definition: a typed, named Contact attribute (ADR 0006) */
model CustomFieldResource {
  /** Unique identifier */
  id: EntityId;

  /** Machine key, how values are addressed in custom fields and segment rules */
  @extension("x-untrusted", true)
  key: string;

  /** Display name */
  @extension("x-untrusted", true)
  name: string;

  /** Inferred value type */
  type: CustomFieldType;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Delivery channel an integration belongs to */
enum IntegrationChannel {
  email,
  sms,
}

/** Concrete sending provider */
enum IntegrationProvider {
  smtp,
  ses,
}

/** Where an effective Send rate limit comes from */
enum SendLimitSource {
  /** Set by the staff member on the Integration */
  manual,

  /** Reported by the provider */
  provider,
}

/** A reason to look at an Integration's Send rate limit */
enum SendLimitWarning {
  /** No ceiling applies: sends are not paced and the provider may throttle or block them */
  unlimited,

  /** The provider's send quota could not be read (no permission, or a compatible service without the call), so only the manual limit applies */
  providerQuotaUnavailable,
}

/** One effective ceiling: the lowest of the manual and provider values, manual on a tie */
model SendLimitValue {
  /** The ceiling in messages; null means this window is not limited */
  limit: int32 | null;

  /** Where the ceiling comes from; null when there is none */
  source: SendLimitSource | null;
}

/** The Send rate limit as enforced, with the last 24 hours of usage */
model SendLimitStatus {
  /** Most messages per second */
  perSecond: SendLimitValue;

  /** Most messages per rolling 24 hours */
  perDay: SendLimitValue;

  /** Messages the provider accepted from this Integration in the last 24 hours */
  sentLast24h: int32;

  /** Reasons to review the limit; empty when there is nothing to flag */
  warnings: SendLimitWarning[];
}

/**
 * A rate as the (numerator, denominator, rate) triple (ADR 0011). The rate is
 * null when the denominator is zero: an undefined rate is never reported as 0.
 */
model RateTriple {
  /** Events counted against the rate */
  numerator: int32;

  /** The population the rate is over */
  denominator: int32;

  /** numerator / denominator, null when the denominator is zero */
  rate: float64 | null;
}

/**
 * Complaint and bounce rates of one Sending domain over a trailing window (ADR 0011).
 * Complaint rate is complaints / (sent - hard bounces); bounce rate is hard bounces / sent.
 */
model SendingDomainRatesResource {
  /** Sending domain id */
  sendingDomainId: EntityId;

  /** The domain the rates are for */
  domain: string;

  /** Trailing window the rates cover, in days */
  windowDays: int32;

  /** Complaints over delivered-ish sends (sent minus hard bounces) */
  complaintRate: RateTriple;

  /** Hard bounces over sent */
  bounceRate: RateTriple;
}

@route("/custom-fields")
@tag("Custom fields")
namespace CustomFields {
  /** List the workspace's custom field definitions */
  @get
  op list():
    | Page<CustomFieldResource>
    | UnauthorizedProblem
    | TooManyRequestsProblem;
}

@route("/sending-domains/rates")
@tag("Sending domains")
namespace SendingDomainRates {
  /** Complaint and bounce rates per sending domain over a trailing window */
  @get
  op list(
    /** Trailing window in days (1 to 90) */
    @query windowDays?: int32 = 7,
  ):
    | Page<SendingDomainRatesResource>
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem;
}
