// The Broadcast write bodies and its content: /site and /api take and show the same.

namespace Sphericon;

/** The Channel a Broadcast sends on, fixed when it is created (ADR 0080) */
enum BroadcastChannel {
  email,
  telegram,
  sms,
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

/**
 * Telegram's content of a Broadcast (ADR 0078), the same Message content as a Telegram step. On
 * create an omitted field takes its default (an empty text, no image, no buttons, the link preview
 * on, no unsubscribe button); on update an omitted field keeps its value.
 */
model TelegramBroadcastContentInput {
  /** The Channel the content is for: the Broadcast's own, which an update cannot change */
  channel: "telegram";

  /** The message text in Telegram's HTML subset (b, i, u, s, a, code, pre, tg-spoiler, blockquote) with Liquid merge tags, at most 4096 characters as written (Schedule refuses a longer one). Nothing is appended to it. */
  text?: string;

  /** An https image sent with the message; the text becomes its caption, of at most 1024 characters as written. An empty string removes the image. */
  imageUrl?: string;

  /** Up to 8 link buttons under the message, one per row, in order. An empty list removes them. */
  @maxItems(8)
  buttons?: TelegramButton[];

  /** Whether a link in the text unfolds a preview. On when omitted; no effect with an image. */
  linkPreview?: boolean;

  /** The label of an unsubscribe button under the message, up to 64 characters. Pressing it is the person's Channel-wide Telegram opt-out. An empty string removes the button. */
  @maxLength(64)
  unsubscribeButton?: string;
}

/**
 * SMS's content of a Broadcast (ADR 0078), the same Message content as an SMS step. On create an
 * omitted field takes its default (an empty text, link tracking on); on update an omitted field
 * keeps its value.
 */
model SmsBroadcastContentInput {
  /** The Channel the content is for: the Broadcast's own, which an update cannot change */
  channel: "sms";

  /** The text, plain, with Liquid merge tags; nothing is appended to it. Schedule refuses a blank one, and a save one over the part limit, its links counted as short links while `linkTracking` is on. */
  text?: string;

  /** Whether each http(s) link of the text goes out as a short tracked link on the Tracking domain, whose clicks record `sms.clicked` (ADR 0083). On when omitted. */
  linkTracking?: boolean;
}

/** What a Broadcast says, in the shape of its Channel, discriminated by `channel` (ADR 0078) */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "channel" })
union BroadcastContentInput {
  email: EmailBroadcastContentInput,
  telegram: TelegramBroadcastContentInput,
  sms: SmsBroadcastContentInput,
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

/** Telegram's content of a Broadcast as stored */
model TelegramBroadcastContent {
  /** The Channel the content is for */
  channel: "telegram";

  /** The message text in Telegram's HTML subset, with Liquid merge tags */
  text: string;

  /** The https image sent with the message, its caption being the text; null for a text message */
  imageUrl?: string | null;

  /** The link buttons under the message, one per row, in order */
  buttons: TelegramButton[];

  /** Whether a link in the text unfolds a preview */
  linkPreview: boolean;

  /** The label of the unsubscribe button under the message; null for none */
  unsubscribeButton?: string | null;
}

/** SMS's content of a Broadcast as stored */
model SmsBroadcastContent {
  /** The Channel the content is for */
  channel: "sms";

  /** The text, plain, with Liquid merge tags */
  text: string;

  /** Whether each link of the text goes out as a short tracked link */
  linkTracking: boolean;
}

/** What a Broadcast says, in the shape of its Channel, discriminated by `channel` (ADR 0078) */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "channel" })
union BroadcastContent {
  email: EmailBroadcastContent,
  telegram: TelegramBroadcastContent,
  sms: SmsBroadcastContent,
}

/** Request body for creating a broadcast */
model CreateBroadcastInput {
  /** Internal broadcast name */
  name: string;

  /** What the broadcast says. Its `channel` is the Channel the broadcast sends on, fixed from now on: a broadcast is never converted to another Channel. An SMS broadcast in a Workspace with no SMS integration is refused with 422. */
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

/**
 * Request body for a test send: where the preview goes, in the field of the broadcast's Channel.
 * The field of the other Channel is not read.
 */
model TestSendBroadcastInput {
  /** Address to send the rendered preview of an email broadcast to; required on email */
  email?: EmailAddress;

  /** Phone number to send the rendered text of an SMS broadcast to, in international format with its country code (+14155550100); required on SMS, and a value that is no such number is a 422 */
  phone?: string;
}
