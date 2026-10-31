namespace Sphericon;

/** How the contact first came to exist */
enum ContactExportSource {
  manual,
  api,
  tracker,
}

/** The contact itself */
model ContactExportContact {
  /** Unique identifier */
  id: EntityId;

  /** Canonical subject identifier */
  subjectId?: string | null;

  /** Email address */
  email?: string | null;

  /** Phone number */
  phone?: string | null;

  /** Given name */
  firstName?: string | null;

  /** Family name */
  lastName?: string | null;

  /** IANA time zone identifier */
  timeZone?: string | null;

  /** Typed custom field values keyed by the field's machine key */
  customFields?: JsonObject | null;

  /** The door the contact came through; absent for a contact that predates the field */
  source?: ContactExportSource | null;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** A tag the contact carries */
model ContactExportTag {
  /** Unique identifier */
  id: EntityId;

  /** The label */
  name: string;

  /** Creation timestamp */
  createdAt: Timestamp;
}

/** An anonymous device (visitor) bound to the contact */
model ContactExportVisitor {
  /** Unique identifier */
  id: EntityId;

  /** The visitor id the tracker assigned */
  visitorId: string;

  /** When the visitor was last seen */
  lastSeenAt: Timestamp;

  /** Creation timestamp */
  createdAt: Timestamp;
}

/** An event tracked for the contact or its visitors */
model ContactExportEvent {
  /** Unique identifier */
  id: EntityId;

  /** Source identifier */
  sourceId?: string | null;

  /** Visitor id the event was tracked under */
  visitorId?: string | null;

  /** Canonical subject identifier */
  subjectId: string;

  /** Email address associated with the event */
  email?: string | null;

  /** Phone number associated with the event */
  phone?: string | null;

  /** Event action */
  action: string;

  /** Event properties */
  properties?: JsonObject | null;

  /** When the event happened: the time the source gave, or when it was published if it gave none */
  occurredAt: Timestamp;

  /** Whether the event was loaded as history rather than tracked live */
  backfill: boolean;

  /** Creation timestamp */
  createdAt: Timestamp;
}

/** An opt-out of a channel destination */
model ContactExportUnsubscribe {
  /** Unique identifier */
  id: EntityId;

  /** Channel the opt-out applies to */
  channel: string;

  /** Address the opt-out applies to */
  destination: string;

  /** The sending source (kind of mail) opted out of */
  sendingSource: string;

  /** Creation timestamp */
  createdAt: Timestamp;
}

/** A suppression of a channel destination */
model ContactExportSuppression {
  /** Unique identifier */
  id: EntityId;

  /** Channel the suppression applies to */
  channel: string;

  /** Address the suppression applies to */
  destination: string;

  /** Why the destination is suppressed */
  reason: string;

  /** When the suppression was lifted; null while it is active */
  liftedAt?: Timestamp | null;

  /** Creation timestamp */
  createdAt: Timestamp;
}

/** A recorded confirmation of a channel destination */
model ContactExportConfirmation {
  /** Unique identifier */
  id: EntityId;

  /** Channel the confirmation applies to */
  channel: string;

  /** Address the confirmation applies to */
  destination: string;

  /** How the confirmation was obtained */
  provenance: string;

  /** Creation timestamp */
  createdAt: Timestamp;
}

/** Delivery metadata of one outbound message; the rendered message is not included */
model ContactExportOutboundMessage {
  /** Unique identifier */
  id: EntityId;

  /** What kind of send produced the message */
  kind: string;

  /** Channel the message was sent over */
  channel: string;

  /** Address the message was sent to */
  destination: string;

  /** Sending source */
  sendingSource?: string | null;

  /** Sending domain */
  sendingDomain?: string | null;

  /** The provider's message id */
  providerMessageId?: string | null;

  /** Delivery status */
  status: string;

  /** Why delivery failed or was skipped */
  reason?: string | null;

  /** When the send was claimed */
  claimedAt: Timestamp;

  /** When the message was sent */
  sentAt?: Timestamp | null;

  /** When the provider first reported the message delivered */
  deliveredAt?: Timestamp | null;

  /** When the provider first reported the message bounced */
  bouncedAt?: Timestamp | null;

  /** When the provider first reported a spam complaint about the message */
  complainedAt?: Timestamp | null;

  /** True when the message went through a Test inbox: captured, not delivered */
  captured: boolean;

  /** Broadcast that produced the message */
  broadcastId?: EntityId | null;

  /** Automation that produced the message */
  automationId?: EntityId | null;

  /** Step of the automation that produced the message */
  automationStep?: int32 | null;

  /** Template the message was rendered from */
  templateId?: EntityId | null;

  /** Creation timestamp */
  createdAt: Timestamp;
}

/** The contact's engagement with one broadcast (how its send ended is in `outboundMessages`) */
model ContactExportBroadcastRecipient {
  /** Unique identifier */
  id: EntityId;

  /** The broadcast */
  broadcastId: EntityId;

  /** When the contact opened it */
  openedAt?: Timestamp | null;

  /** When the contact clicked it */
  clickedAt?: Timestamp | null;

  /** When the contact unsubscribed from it */
  unsubscribedAt?: Timestamp | null;

  /** Creation timestamp */
  createdAt: Timestamp;
}

/**
 * Everything held about one contact (subject access, GDPR Art. 15/20). The
 * download is this document streamed member by member; rendered message bodies
 * are never part of it.
 */
model ContactExportDocument {
  /** The contact */
  contact: ContactExportContact;

  /** Tags the contact carries */
  tags: ContactExportTag[];

  /** Visitors bound to the contact */
  visitors: ContactExportVisitor[];

  /** Opt-outs for the contact's addresses */
  unsubscribes: ContactExportUnsubscribe[];

  /** Suppressions for the contact's addresses */
  suppressions: ContactExportSuppression[];

  /** Confirmations for the contact's addresses */
  confirmations: ContactExportConfirmation[];

  /** All events of the contact and its visitors */
  events: ContactExportEvent[];

  /** Delivery metadata of messages sent to the contact's addresses */
  outboundMessages: ContactExportOutboundMessage[];

  /** The contact's recipient rows in broadcasts */
  broadcastRecipients: ContactExportBroadcastRecipient[];
}
