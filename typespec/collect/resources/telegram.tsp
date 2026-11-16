using TypeSpec.Http;

namespace Sphericon;

model CollectTelegramStartLinkInput {
  /** The tracker's id of the visitor the link is for, created when it has not been seen */
  @minLength(1)
  @maxLength(255)
  visitorId: string;

  /** An identity token: a JWT the site's server signs with HS256 under the workspace's identity
   * secret, naming the signed-in person's keys (`sub` for the subjectId, `email`, `phone`) and an
   * `exp` at most a day ahead. A token that does not verify refuses the call with 422. With it the link is for the contact the token proves, when there is one */
  @maxLength(4096)
  identityToken?: string | null;
}

/** A Telegram start link for a visitor: pressing Start in the bot binds the chat to the visitor, which reaches no contact until a proven identify hands it over, or to the contact an identity token proves. The link works once. */
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
