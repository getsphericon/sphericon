using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/**
 * One row of the Erasures stream: the keys one Erasure matched on, so a consumer deletes the
 * same rows from its copy. Match on any of them: `contact_id` on Events and Contacts,
 * `subject_id` on Events, `visitor_ids` on Events, `email_hash` and `phone_hash` against
 * the SHA-256 (hex) of the lowercased address or of the phone number as stored. The address
 * and the phone themselves are never part of the stream. A row is never changed once written.
 */
model ExportErasureRow {
  /** Unique identifier */
  id: EntityId;

  /** The erased Contact; absent when the Erasure named an address or an anonymous Visitor with no Contact */
  contact_id: EntityId | null;

  /** The customer's own id for the person (the Contact's `subject_id`, or its Sphericon id when it had none); empty when there was none */
  subject_id: string;

  /** The anonymous devices erased with the Contact */
  visitor_ids: string[];

  /** SHA-256, as lowercase hex, of the lowercased email address; absent when there was none */
  email_hash: string | null;

  /** SHA-256, as lowercase hex, of the phone number; absent when there was none */
  phone_hash: string | null;

  /** When the Erasure happened */
  erased_at: Timestamp;

  /** Creation timestamp */
  created_at: Timestamp;

  /** Last change timestamp: the cursor column and the version of the row */
  updated_at: Timestamp;
}

@route("/export")
@tag("Export")
namespace Export {
  /**
   * Read the Workspace's Erasures, oldest first (Enterprise; needs the `export:read` scope).
   * Each line of the body is an `ExportErasureRow`; every key is always present. Rows live for
   * the instance's Event retention window and are kept for good when retention is off: a
   * consumer absent for longer re-seeds from an empty cursor.
   */
  @get
  @route("/erasures")
  @extension("x-required-scopes", #[ApiTokenScope.exportRead])
  op erasures(...ExportPageQuery):
    | ExportPage
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | NotFoundProblem;
}
