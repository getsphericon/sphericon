using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** Event payload */
model EventInput {
  /** Canonical subject identifier */
  subjectId: string;

  /** Event action */
  @untrusted
  action: string;

  /** Email address associated with the event */
  email?: EmailAddress | null;

  /** Phone number associated with the event */
  phone?: string | null;

  /** Event properties */
  @untrusted
  properties?: Record<unknown> | null;

  /** Event occurrence timestamp */
  occurredAt?: Timestamp | null;

  /**
   * A client-chosen id that makes a retry safe. It is unique within the Workspace: sending an
   * event again with the same id changes nothing, so a load that broke off halfway is resent
   * whole. Generate it once per event (a UUID) and reuse it on every retry. An event without
   * one is not deduplicated.
   */
  @minLength(1)
  @maxLength(255)
  clientEventId?: string | null;
}

/** Request body for recording events */
model RecordEventsInput {
  /** Events to record */
  events: EventInput[];
}

/** Request body for recording events one by one with per-item results */
model RecordEventsBatchInput {
  /**
   * Load history: the events are only recorded, so segments see them but no Automation
   * enrolls on them and no webhook endpoint receives them. Use it to bring the events of
   * an earlier system into a new setup. Defaults to false.
   */
  backfill?: boolean;

  /** Events to record, at most 1000 */
  @minItems(1)
  @maxItems(1000)
  events: EventInput[];
}

/** Outcome of one item of an events batch */
enum EventBatchStatus {
  accepted,
  failed,
}

/** Per-item result of an events batch, in request order */
model EventBatchItemResult {
  /** Zero-based position of the item in the request */
  index: int32;

  /** accepted when published; failed when the item was rejected */
  status: EventBatchStatus;

  /** Why the item failed; absent when accepted */
  error?: string;
}

/** Per-item results of an events batch */
model RecordEventsBatchResult {
  /** One result per request item, in request order */
  results: EventBatchItemResult[];
}

/** Distinct event action */
model EventActionResource {
  /** Event action */
  @untrusted
  action: string;
}

@route("/events")
@tag("Events")
namespace Events {
  /** Record events in batch */
  @post
  @requiresScopes(#[ApiTokenScope.eventsWrite])
  op create(@body body: RecordEventsInput):
    | NoContentResponse
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | BadRequestProblem
    | UnprocessableEntityProblem;
}

// Aligned with the site API's `/events/actions` naming.
@route("/events/actions")
@tag("Events")
namespace EventActions {
  /** List unique event actions */
  @get
  @requiresScopes(#[ApiTokenScope.eventsRead])
  op list(...PageQuery):
    | Page<EventActionResource>
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | UnprocessableEntityProblem;
}

@route("/events/batch")
@tag("Events")
namespace EventsBatch {
  /** Record up to 1000 events; a failing item does not affect the others */
  @post
  @requiresScopes(#[ApiTokenScope.eventsWrite])
  op submit(@body body: RecordEventsBatchInput):
    | RecordEventsBatchResult
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | BadRequestProblem
    | UnprocessableEntityProblem;
}
