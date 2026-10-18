using TypeSpec.Http;

namespace Sphericon;

model CollectEventInput {
  @minLength(1)
  @maxLength(255)
  visitorId: string;

  @minLength(1)
  @maxLength(255)
  action: string;

  /**
   * A client-chosen id that makes a retry safe. It is unique within the Workspace: sending
   * an event again with the same id changes nothing, so a client that did not see the answer
   * (a network error, a `503`) resends the whole batch. Generate it once per event, when the
   * event happens (a UUID), and reuse it on every retry. An event without one is not deduplicated.
   */
  @minLength(1)
  @maxLength(255)
  clientEventId?: string | null;

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
    | UnprocessableEntityProblem
    | ServiceUnavailableProblem;
}
