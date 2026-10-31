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

  traits?: JsonObject | null;
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
