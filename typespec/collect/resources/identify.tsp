using TypeSpec.Http;

namespace Sphericon;

/** An identify claims who the visitor is: it finds or creates the contact by its keys, but gives a
 * contact that exists no key it lacks and replaces none of its values, and binds the visitor only
 * while it belongs to no other contact. With an identity token it is proven: the token's keys are
 * the contact's, it fills keys and replaces values, moves the visitor and hands the contact the
 * chats the visitor started */
model CollectIdentifyInput {
  @minLength(1)
  @maxLength(255)
  visitorId: string;

  email?: EmailAddress | null;

  /** Phone number — an alias key, stored in E.164. A number without a country code is read with
   * the `country` trait or the country of the contact the other keys find; one that does not
   * parse is refused */
  @maxLength(32)
  phone?: string | null;

  @maxLength(255)
  subjectId?: string | null;

  /** An identity token: a JWT the site's server signs with HS256 under the workspace's identity
   * secret, naming the signed-in person's keys (`sub` for the subjectId, `email`, `phone`) and an
   * `exp` at most a day ahead. A token that does not verify refuses the call with 422. A key sent beside it must be the token's */
  @maxLength(4096)
  identityToken?: string | null;

  /** Attributes of the contact; a trait named after a core attribute (`firstName`, `timeZone`,
   * `country`, `locale`, `birthday`, `city`, `avatar`) writes it, and an invalid value of one is
   * dropped. Any other trait writes the custom field of its key when the workspace has defined one,
   * and is dropped otherwise: the collect key defines none. A date is an ISO string
   * (`2026-01-15T10:00:00Z`, `2026-01-15T10:00:00` or `2026-01-15`, the last two read in the
   * Workspace time zone): send a date without time as a `YYYY-MM-DD` string, not a JavaScript
   * `Date`, which serializes as UTC and lands on the previous day east of UTC. The call proves
   * nothing, so on a contact that exists it only fills what the contact has no value of */
  traits?: JsonObject | null;

  /** What the browser tells of the visitor: it fills only a time zone or locale the contact has
   * none of */
  context?: CollectIdentifyContext | null;
}

/** The browser's own settings, sent by the tracker with an identify */
model CollectIdentifyContext {
  /** The browser's IANA time zone */
  @maxLength(64)
  timeZone?: string | null;

  /** The browser's language, a BCP 47 tag */
  @maxLength(35)
  locale?: string | null;
}

model CollectOkResponse {
  ok: true;
}

@route("/identify")
@tag("Collect")
namespace CollectIdentify {
  @post
  op create(@body body: CollectIdentifyInput): CollectOkResponse | Problem;
}
