// What a Broadcast's or an Automation's messages earned and how their links were clicked: /site
// and /api show the same.

namespace Sphericon;

/** The Orders credited to one Broadcast or Automation and the net revenue of their Movements
 * (ADR 0064), in the Reporting currency, read from the Movements and never stored: a refund
 * lowers `amount` on its own day. Orders in another currency are counted in neither field. */
model MessageRevenue {
  /** The Reporting currency, null until the first order or an owner or admin sets it; every figure is zero until then */
  reportingCurrency: CurrencyCode | null;

  /** Orders completed that are credited to it, in the Reporting currency */
  orders: int32;

  /** Net revenue of those Orders, in the minor unit of the Reporting currency (cents for USD) */
  amount: int64;
}

/** How often one link of a broadcast's or an automation's emails was clicked */
model LinkClicks {
  /** The link's destination URL */
  url: string;

  /** Clicks recorded on it, every click counted (not unique per contact); bounded by Event retention */
  clicks: int32;
}
