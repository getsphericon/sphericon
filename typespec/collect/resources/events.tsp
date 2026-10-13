using TypeSpec.Http;

namespace Sphericon;

model CollectEventInput {
  @minLength(1)
  @maxLength(255)
  visitorId: string;

  @minLength(1)
  @maxLength(255)
  action: string;

  properties?: Record<unknown> | null;
  occurredAt?: Timestamp | null;
}

model CollectEventsInput {
  @maxItems(100)
  events: CollectEventInput[];
}

@route("/events")
@tag("Collect")
namespace CollectEvents {
  @post
  op create(@body body: CollectEventsInput):
    | NoContentResponse
    | UnauthorizedProblem
    | PayloadTooLargeProblem
    | TooManyRequestsProblem
    | UnprocessableEntityProblem;
}
