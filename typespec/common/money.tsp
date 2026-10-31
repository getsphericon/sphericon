namespace Sphericon;

// Money is shared by the surfaces that report revenue (ADR 0064), /site and /api, and only they
// import it: a scalar in scalars.tsp is emitted into every spec.

/** An ISO 4217 alphabetic currency code in capitals, such as `EUR`. Which codes exist is decided
 * by the server, which refuses an unknown one with a reason. */
@pattern("^[A-Z]{3}$")
scalar CurrencyCode extends string;
