namespace Sphericon;

/** A Channel whose messages track opens and clicks, so a Destination on it can consent to being tracked (ADR 0120) */
enum EngagementTrackingChannel {
  email,
  sms,
  telegram,
}

/** Whether a Destination agreed to have its opens and clicks tracked (ADR 0120) */
enum EngagementTrackingState {
  granted,
  denied,
}

/** Who recorded an Engagement tracking consent: the person on a page reached from a message (page), a member of the Workspace on the Contact card (manual), the customer through the API or an import (imported), or an On-site form's checkbox (onsite_form). A `denied` applies from any of them; only the person lifts one */
enum EngagementTrackingProvenance {
  page,
  manual,
  imported,
  onsite_form,
}

/** What a Destination's Engagement tracking consent reads as: `granted` or `denied` as recorded, or `default` where nothing is recorded and the Workspace's default decides */
enum EngagementTrackingReading {
  granted,
  denied,
  default,
}

/** Whether a Destination's opens and clicks are tracked */
enum EngagementTrackingDecision {
  track,
  dont_track,
}

/** The Engagement tracking consent of one Destination on one Channel (ADR 0120) */
model EngagementTrackingConsent {
  /** The Channel */
  channel: EngagementTrackingChannel;

  /** Normalized destination: the lower-cased email address on `email` */
  destination: string;

  /** The consent recorded, or `default` where none is */
  state: EngagementTrackingReading;

  /** Who recorded the consent; absent while none is recorded */
  provenance?: EngagementTrackingProvenance | null;

  /** Whether the Destination's opens and clicks are tracked: what the consent says, else the Workspace's default */
  effective: EngagementTrackingDecision;

  /** When the consent last changed; absent while none is recorded */
  updatedAt?: Timestamp | null;
}

/** The Engagement tracking consent of each Destination of a contact (its email address) */
model ContactEngagementTracking {
  /** One entry per Destination of the contact; empty when it has none */
  destinations: EngagementTrackingConsent[];
}
