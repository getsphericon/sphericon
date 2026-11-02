// The Telegram start link of a Contact: /site and /api take the same input and answer the same
// resource.

namespace Sphericon;

/** Request body for issuing a Telegram start link */
model IssueTelegramStartLinkInput {
  /** The Telegram bot the link is for; null or omitted means the workspace's default Telegram integration. A disabled or archived bot, or an integration of another channel, is refused with 422. */
  integrationId?: EntityId | null;
}

/** A Telegram start link: the person who opens it and presses Start binds their chat to the contact it was issued for. The link works once. */
model TelegramStartLinkResource {
  /** `https://t.me/<bot>?start=<token>`. The token is shown only here: treat the link as a secret until it is used. */
  url: string;

  /** When the link lapses, 30 days after it is issued */
  expiresAt: Timestamp;
}
