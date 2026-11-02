// The Broadcast write bodies and its content: /site and /api take and show the same.

namespace Sphericon;

/** The Channel a Broadcast sends on, fixed when it is created (ADR 0080) */
enum BroadcastChannel {
  email,
}

/**
 * Email's content of a Broadcast (ADR 0078). On create an omitted field takes its default; on
 * update an omitted field keeps its value and null clears it.
 */
model EmailBroadcastContentInput {
  /** The Channel the content is for: the Broadcast's own, which an update cannot change */
  channel: "email";

  /** Email subject line */
  subject?: string;

  /** Sender display name; null or omitted on create means the integration's default, null on update goes back to it */
  fromName?: string | null;

  /** Sender email address: a new value must be on a verified Sending domain of the workspace, or the write is a 422. An unchanged one is not re-checked, so a draft whose domain lost verification stays editable. Null or omitted on create means the integration's default, null on update goes back to it. */
  fromEmail?: EmailAddress | null;

  /** HTML body with Liquid merge tags. Writing only a body on update clears the stored document. Write this or `doc`, not both. */
  body?: string;

  /** The editor's JSON document, an opaque object checked by the server against the schema `email_schema` returns. The server renders the
   * body from it in the same save; a document that cannot be rendered is a 422 with the reason and nothing is stored. */
  doc?: EditorDocument;
}

/** What a Broadcast says, in the shape of its Channel, discriminated by `channel` (ADR 0078) */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "channel" })
union BroadcastContentInput {
  email: EmailBroadcastContentInput,
}

/** Email's content of a Broadcast as stored */
model EmailBroadcastContent {
  /** The Channel the content is for */
  channel: "email";

  /** Email subject line */
  subject: string;

  /** Sender display name (defaults to the integration's) */
  fromName?: string | null;

  /** Sender email address (defaults to the integration's) */
  fromEmail?: EmailAddress | null;

  /** HTML body with Liquid merge tags */
  body: string;

  /** The editor's JSON document the body was built from; null when the body was written as HTML */
  doc?: EditorDocument | null;

  /** Version of the editor that rendered the document; null with it */
  editorVersion?: string | null;
}

/** What a Broadcast says, in the shape of its Channel, discriminated by `channel` (ADR 0078) */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "channel" })
union BroadcastContent {
  email: EmailBroadcastContent,
}

/** Request body for creating a broadcast */
model CreateBroadcastInput {
  /** Internal broadcast name */
  name: string;

  /** What the broadcast says. Its `channel` is the Channel the broadcast sends on, fixed from now on: a broadcast is never converted to another Channel. */
  content: BroadcastContentInput;

  /** Target segment; null or omitted means all contacts. An archived segment is refused with 422. */
  segmentId?: EntityId | null;

  /** Sending integration, of the broadcast's Channel; null or omitted means the Channel's default. An archived integration, or one of another Channel, is refused with 422. */
  integrationId?: EntityId | null;
}

/** Request body for updating a draft broadcast: an omitted field keeps its value, null clears it */
model UpdateBroadcastInput {
  /** Internal broadcast name */
  name?: string;

  /** What the broadcast says; its `channel` must be the broadcast's own (422 otherwise). Omitted keeps the content. */
  content?: BroadcastContentInput;

  /** Target segment; null means all contacts. An archived segment is refused with 422. */
  segmentId?: EntityId | null;

  /** Sending integration, of the broadcast's Channel; null means the Channel's default. An archived integration, or one of another Channel, is refused with 422. */
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
