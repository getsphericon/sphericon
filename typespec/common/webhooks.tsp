// The Webhook endpoint write bodies: /site and /api take the same input.

namespace Sphericon;

/** Request body for creating a webhook endpoint */
model CreateWebhookInput {
  /** Destination URL (absolute http or https) */
  url: string;

  /** Event names to receive; empty or absent means all events */
  eventTypes?: string[];

  /** Whether the endpoint receives deliveries (default true) */
  enabled?: boolean;
}

/** Request body for updating a webhook endpoint */
model UpdateWebhookInput {
  /** Destination URL (absolute http or https) */
  url?: string;

  /** Event names to receive; empty means all events */
  eventTypes?: string[];

  /** Whether the endpoint receives deliveries */
  enabled?: boolean;
}

/** Request body for sending a sample event to a webhook endpoint */
model TestWebhookInput {
  /** The event name the sample carries, for example `contact.created` */
  eventType: string;
}
