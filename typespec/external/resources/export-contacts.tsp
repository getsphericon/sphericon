using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** One row of the Contacts stream. A row is delivered again whenever it changes: a column or Custom field edit, a Tag attached or detached, an Erasure's anonymization */
model ExportContactRow {
  /** Unique identifier */
  id: EntityId;

  /** The customer's own user id; absent when the Contact has none */
  subject_id: string | null;

  /** Email address; absent for an anonymous Contact */
  email: string | null;

  /** Phone number; absent when the Contact has none */
  phone: string | null;

  /** First name */
  first_name: string | null;

  /** Last name */
  last_name: string | null;

  /** IANA time zone identifier */
  time_zone: string | null;

  /** Custom field values keyed by the field's machine key, as one JSON column */
  custom_fields: ContactExportJson;

  /** Names of the Tags the Contact has, ordered by name */
  tags: string[];

  /** Creation timestamp */
  created_at: Timestamp;

  /** Last change timestamp, a Tag change included: the cursor column and the version of the row */
  updated_at: Timestamp;
}

namespace Export {
  /**
   * Read the Workspace's Contacts, oldest change first (Enterprise; needs the `export:read`
   * scope). Each line of the body is an `ExportContactRow`; every key is always present.
   */
  @get
  @route("/contacts")
  @extension("x-required-scopes", #[ApiTokenScope.exportRead])
  op contacts(...ExportPageQuery):
    | ExportPage
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | NotFoundProblem;
}
