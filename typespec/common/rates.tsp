// A rate of the Sending domains' reputation: /site and /api show the same.

namespace Sphericon;

/** A rate as the (numerator, denominator, rate) triple (ADR 0011), shown as it is, with no
 * display floor: the rate is null only when the denominator is zero, an undefined rate never
 * being reported as 0. */
model RateTriple {
  /** Events counted against the rate */
  numerator: int32;

  /** The population the rate is over */
  denominator: int32;

  /** numerator / denominator, null when the denominator is zero, never reported as 0 */
  rate: float64 | null;
}
