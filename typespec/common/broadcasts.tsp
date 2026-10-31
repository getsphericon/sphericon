// The Broadcast write bodies: /site and /api take the same input.

namespace Sphericon;

/** Request body for creating a broadcast */
model CreateBroadcastInput {
  /** Internal broadcast name */
  name: string;

  /** Email subject line */
  subject?: string;

  /** Sender display name; null or omitted means the integration's default */
  fromName?: string | null;

  /** Sender email address: its domain must be a verified Sending domain of the workspace, or the write is a 422. Null or omitted means the integration's default. */
  fromEmail?: EmailAddress | null;

  /** HTML body with Liquid merge tags. Write this or `doc`, not both. */
  body?: string;

  /** The editor's JSON document, an opaque object checked by the server against the schema `email_schema` returns. The server renders the
   * body from it in the same save; a document that cannot be rendered is a 422 with the reason and nothing is stored. */
  doc?: Record<unknown>;

  /** Target segment; null or omitted means all contacts. An archived segment is refused with 422. */
  segmentId?: EntityId | null;

  /** Sending integration; null or omitted means the workspace default. An archived integration is refused with 422. */
  integrationId?: EntityId | null;
}

/** Request body for updating a draft broadcast: an omitted field keeps its value, null clears it */
model UpdateBroadcastInput {
  /** Internal broadcast name */
  name?: string;

  /** Email subject line */
  subject?: string;

  /** Sender display name; null goes back to the integration's default */
  fromName?: string | null;

  /** Sender email address: a changed value must be on a verified Sending domain of the workspace, or the write is a 422. An unchanged one is not re-checked, so a draft whose domain lost verification stays editable. Null goes back to the integration's default. */
  fromEmail?: EmailAddress | null;

  /** HTML body. Writing only a body clears the stored document. Write this or `doc`, not both. */
  body?: string;

  /** The editor's JSON document, an opaque object checked by the server against the schema `email_schema` returns. The server renders the
   * body from it in the same save; a document that cannot be rendered is a 422 with the reason and nothing is stored. */
  doc?: Record<unknown>;

  /** Target segment; null means all contacts. An archived segment is refused with 422. */
  segmentId?: EntityId | null;

  /** Sending integration; null means the workspace default. An archived integration is refused with 422. */
  integrationId?: EntityId | null;
}

/** Request body for scheduling a broadcast */
model ScheduleBroadcastInput {
  /** When the broadcast should be sent */
  scheduledAt: Timestamp;
}

/** Request body for a test send */
model TestSendBroadcastInput {
  /** Address to send the rendered preview to */
  email: EmailAddress;
}
