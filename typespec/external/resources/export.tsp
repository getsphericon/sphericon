using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/**
 * One page of an Export stream (Enterprise, ADR 0045): newline-delimited JSON, one row
 * per line with no envelope, so the body loads as ClickHouse `JSONEachRow`. The row
 * schema is named by each operation. The next cursor is the `X-Next-Cursor` header;
 * pass it as `cursor` for the next page. The cursor is the position after the last row of
 * the page and the next page includes everything at or after it, so rows that share an
 * `updated_at` across a page edge are not lost. A page with no rows returns the cursor it
 * was given. A row that changes is delivered again with a newer `updated_at`: keep the
 * newest version per `id`.
 */
model ExportPage {
  @header contentType: "application/x-ndjson";

  /** Opaque cursor for the next page */
  @header("X-Next-Cursor") nextCursor: string;

  @body body: bytes;
}

/** The query parameters every Export stream takes */
model ExportPageQuery {
  /** Cursor from the `X-Next-Cursor` header of a previous page; empty reads from the beginning */
  @query cursor?: string;

  /** Page size */
  @query
  @minValue(1)
  @maxValue(10000)
  limit?: int32 = 1000;
}

/** One row of the Events stream. A row is delivered again whenever it changes (an anonymous Event stitched onto a Contact, an `email.*` Event anonymized by an Erasure) */
model ExportEventRow {
  /** Unique identifier */
  id: EntityId;

  /** Event action */
  action: string;

  /** Event properties as one JSON column */
  properties: JsonObject;

  /** The Contact the Event is about; absent for an anonymous Event */
  contact_id: EntityId | null;

  /** The anonymous device the Event came from */
  visitor_id: string | null;

  /** Canonical subject identifier snapshot; empty for an anonymous Event */
  subject_id: string;

  /** Email address snapshot; absent once anonymized */
  email: string | null;

  /** Phone number snapshot; absent once anonymized */
  phone: string | null;

  /** When the Event happened: the time the sender gave, or when it was published if it gave none */
  occurred_at: Timestamp;

  /** Creation timestamp */
  created_at: Timestamp;

  /** Last change timestamp: the cursor column and the version of the row */
  updated_at: Timestamp;
}

@route("/export")
@tag("Export")
namespace Export {
  /**
   * Read the Workspace's Events, oldest change first (Enterprise; needs the `export:read`
   * scope). Each line of the body is an `ExportEventRow`; every key is always present.
   */
  @get
  @route("/events")
  @requiresScopes(#[ApiTokenScope.exportRead])
  op events(...ExportPageQuery):
    | ExportPage
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | NotFoundProblem;
}
