// The CSV export of a Segment's contacts, or of all of them (ADR 0138): /site only. A Connection's
// contacts:export reaches the one-contact export of /api; bulk reads for a warehouse are Export
// streams.

using TypeSpec.Http;

namespace Sphericon;

/**
 * A choice of columns of the CSV export of contacts. A key or a core attribute is one column, under
 * its own name. `customFields` is a column per custom field of the workspace, `customFields.<key>`,
 * in the order of the keys. `tags` is one column holding the contact's tag names as one CSV record.
 * A channel (`email`, `sms`, `telegram`) is six columns of its state as a broadcast meets it:
 * `<channel>.state` (`may_receive`, `needs_confirmation`, `opted_out`, `suppressed`, empty for no
 * destination), `<channel>.optedOutScopes` (one CSV record of the scopes opted out of:
 * `everything`, `broadcasts`, `automation:<id>`), `<channel>.optedOutAt` (the latest of them),
 * `<channel>.suppressionReason`, `<channel>.confirmation` (its provenance) and
 * `<channel>.confirmedAt`. Times are RFC 3339 in UTC. The headers are the import's own, so an
 * export maps onto an import by itself.
 */
enum ContactCsvColumn {
  subjectId,
  email,
  phone,
  firstName,
  lastName,
  timeZone,
  country,
  locale,
  birthday,
  city,
  avatar,
  source,
  createdAt,
  updatedAt,
  customFields,
  tags,
  emailChannel,
  smsChannel,
  telegramChannel,
}

/** What to export: a segment's contacts, or all of them, with the columns chosen */
model SiteContactCsvExportInput {
  /** The segment whose contacts are exported, evaluated as the file is read; every contact of the workspace when absent */
  segmentId?: EntityId;

  /** The columns, in the order the file has them; a column named twice comes once. At least one. */
  columns: ContactCsvColumn[];
}

/** A one-time link to the file: it works once, for the session that asked for it, within a minute */
model SiteContactCsvExportLink {
  /** The link to follow in the browser, a path of this server */
  url: string;

  /** When the link lapses */
  expiresAt: Timestamp;
}

/** The CSV file of the export, streamed as it is read */
model SiteContactCsvExportFile {
  @header contentType: "text/csv";
  @header("Content-Disposition") contentDisposition: string;
  @body body: bytes;
}

@route("/workspaces/{workspaceId}/contact-exports")
@tag("Site Contacts")
namespace SiteContactCsvExports {
  /**
   * Ask for the CSV export of a segment's contacts, or of all of them, with the columns chosen.
   * The answer is a one-time link the browser follows to download the file, which is written as it
   * is read: no file is kept. An unknown segment is a 404, no column a 422. Owners and admins only.
   */
  @post
  @requiresPermissions(#[Permission.contactsExport])
  op create(
    @path workspaceId: WorkspaceId,
    @body body: SiteContactCsvExportInput,
  ):
    | {
        @statusCode statusCode: 201;
        @body body: SiteContactCsvExportLink;
      }
    | Problem;

  /**
   * Download the CSV export a link was issued for. The link works once, for the session that asked
   * for it, within a minute of being issued; any other is a 400. The export is recorded in the
   * Audit log as it starts. Owners and admins only.
   */
  @get
  @route("/{token}")
  @requiresPermissions(#[Permission.contactsExport])
  op download(
    @path workspaceId: WorkspaceId,

    /** The token of the link */
    @path token: string,
  ): SiteContactCsvExportFile | Problem;
}
