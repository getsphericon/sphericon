// The pieces of a Telegram message's content (ADR 0078) the Automation step and the Broadcast share.

namespace Sphericon;

/** A link button under a Telegram message: an inline URL button on a row of its own */
model TelegramButton {
  /** The button's label */
  label: string;

  /** The URL it opens: absolute, http or https */
  url: string;
}
