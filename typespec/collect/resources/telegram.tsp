using TypeSpec.Http;

namespace Sphericon;

model CollectTelegramStartLinkInput {
  /** The tracker's id of the visitor the link is for, created when it has not been seen */
  @minLength(1)
  @maxLength(255)
  visitorId: string;
}

/** A Telegram start link for a visitor: pressing Start in the bot binds the chat to the visitor, and it reaches the contact once the visitor is identified. The link works once. */
model CollectTelegramStartLink {
  /** `https://t.me/<bot>?start=<token>` */
  url: string;

  /** When the link lapses, 30 days after it is issued */
  expiresAt: Timestamp;
}

@route("/telegram-start-links")
@tag("Collect")
namespace CollectTelegramStartLinks {
  /** Issues a start link on the workspace's default Telegram integration. It is refused with 422 when the workspace has no enabled default one. */
  @post
  op create(@body body: CollectTelegramStartLinkInput):
    | CollectTelegramStartLink
    | Problem;
}
