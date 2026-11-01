using TypeSpec.Http;

namespace Sphericon;

model CollectIdentifyInput {
  @minLength(1)
  @maxLength(255)
  visitorId: string;

  email?: EmailAddress | null;

  @maxLength(32)
  phone?: string | null;

  @maxLength(255)
  subjectId?: string | null;

  /** Attributes of the contact; a trait named after a core attribute (`firstName`, `timeZone`,
   * `country`, `locale`, `birthday`, `city`, `avatar`) writes it, null clears it, and an invalid
   * value of one is dropped */
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
  op create(@body body: CollectIdentifyInput):
    | CollectOkResponse
    | BadRequestProblem
    | UnauthorizedProblem
    | PayloadTooLargeProblem
    | TooManyRequestsProblem
    | UnprocessableEntityProblem
    | ServiceUnavailableProblem;
}
