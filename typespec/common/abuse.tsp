// The warning tier of the abuse detector (ADR 0026): /site and the Staff console show the same.

namespace Sphericon;

/** The metric of a Sending domain that is over a warning limit of the abuse detector */
enum AbuseMetric {
  complaint,
  bounce,
}

/** A Sending domain in the warning tier of the abuse detector (ADR 0026): its rate is
 * over the warning limit but under the freeze limit. Computed live from the trailing
 * 24 hours since any clearance, never stored. */
model AbuseWarning {
  /** The Sending domain over the limit */
  domain: string;

  /** Which rate is over */
  metric: AbuseMetric;

  /** The rate, 0–1 */
  rate: float64;
}
