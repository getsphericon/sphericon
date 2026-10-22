namespace Sphericon;

/** How often one link of a broadcast's or an automation's emails was clicked */
model SiteLinkClicks {
  /** The link's destination URL */
  url: string;

  /** Clicks recorded on it, every click counted (not unique per contact); bounded by Event retention */
  clicks: int32;
}
