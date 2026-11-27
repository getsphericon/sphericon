using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** An alias key a column of an import fills, by the contract's name (ADR 0002) */
enum ContactImportKey {
  subjectId,
  email,
  phone,
}

/** A core attribute a column of an import writes, by the contract's name (ADR 0072) */
enum ContactImportAttribute {
  firstName,
  lastName,
  timeZone,
  country,
  locale,
  city,
  avatar,
  birthday,
}

/**
 * How a column writes its dates, which the Member chooses (ADR 0138): a date by the order of its
 * day, month and year and their separator, or `rfc3339` for a date and time with its offset. A date
 * without an offset is read in the Workspace time zone.
 */
enum ContactImportDateFormat {
  isoDate: "yyyy-mm-dd",
  dayMonthYearSlash: "dd/mm/yyyy",
  dayMonthYearDot: "dd.mm.yyyy",
  dayMonthYearDash: "dd-mm-yyyy",
  monthDayYearSlash: "mm/dd/yyyy",
  monthDayYearDot: "mm.dd.yyyy",
  monthDayYearDash: "mm-dd-yyyy",
  rfc3339: "rfc3339",
}

/** The character a file's numbers write their decimals after */
enum ContactImportDecimalSeparator {
  point: ".",
  comma: ",",
}

/** The character a file's cells are separated by, detected at upload */
enum ContactImportDelimiter {
  comma,
  semicolon,
  tab,
}

/**
 * Where an import stands: `uploaded` until a mapping is saved, `ready` to run, `running` while its
 * job writes the rows, then `done`; `cancelled` and `failed` end it early, keeping what it wrote.
 */
enum ContactImportState {
  uploaded,
  ready,
  running,
  cancelled,
  failed,
  done,
}

/** A column whose cells fill an alias key */
model ContactImportKeyColumn {
  kind: "key";

  /** The alias key the cells fill */
  key: ContactImportKey;
}

/** A column whose cells write a core attribute */
model ContactImportAttributeColumn {
  kind: "attribute";

  /** The attribute the cells write */
  attribute: ContactImportAttribute;

  /** How the cells write a date: required for `birthday`, null for any other attribute */
  dateFormat: ContactImportDateFormat | null;
}

/** A column whose cells write a Custom field the Workspace has, by its key */
model ContactImportCustomFieldColumn {
  kind: "customField";

  /** The Custom field's key */
  key: string;

  /** How the cells write a date: required for a `datetime` field, null for any other type */
  dateFormat: ContactImportDateFormat | null;
}

/** A column whose cells name the Tags a row adds, separated by commas; a Tag holding a comma is quoted */
model ContactImportTagsColumn {
  kind: "tags";
}

/** A column the import does not read */
model ContactImportIgnoreColumn {
  kind: "ignore";
}

/** The Channel of a refusal column: its Destination is the row's email address or phone, or on Telegram the users of the Contact's Bindings */
enum ContactImportRefusalChannel {
  email,
  sms,
  telegram,
}

/** The Channel of a consent or Engagement tracking column: its Destination is the row's email address or phone */
enum ContactImportConsentChannel {
  email,
  sms,
}

/**
 * What a value of a refusal column records: an Unsubscribe of `everything` or of `broadcasts`
 * (email only), recorded as imported, which only the person can lift; or a Suppression of the
 * `manual` reason, which needs `suppressions:manage`
 */
enum ContactImportRefusal {
  unsubscribeEverything,
  unsubscribeBroadcasts,
  suppression,
}

/** A cell value of a refusal column and what it records */
model ContactImportRefusalValue {
  /** The value, matched ignoring case and the spaces around it, as a whole cell or as one item of a cell holding a comma-separated list */
  @minLength(1)
  @maxLength(255)
  value: string;

  refusal: ContactImportRefusal;
}

/**
 * A column of the old platform's refusals: a row whose cell is one of the listed values records
 * that refusal at its Destination on the Channel, whether its Contact is new or not; any other
 * value records nothing
 */
model ContactImportRefusalColumn {
  kind: "refusal";
  channel: ContactImportRefusalChannel;

  /** The values that mean a refusal, each with what it records */
  @minItems(1)
  @maxItems(50)
  values: ContactImportRefusalValue[];
}

/**
 * The evidence of the consent a consent column records (ADR 0077): each part given once for every
 * row as text, or read from the column it names on each row, exactly one of the two
 */
model ContactImportConsentEvidence {
  /** The consent wording the person was shown and agreed to; null when a column holds it */
  @maxLength(2000)
  wording: string | null;

  /** The column, counted from 0, whose cell is the wording; null when the wording is given */
  wordingColumn: int32 | null;

  /** Where the consent was collected; null when a column holds it */
  @maxLength(500)
  source: string | null;

  /** The column, counted from 0, whose cell is the source; null when the source is given */
  sourceColumn: int32 | null;

  /** The brand the person agreed to hear from; null when a column holds it */
  @maxLength(255)
  brand: string | null;

  /** The column, counted from 0, whose cell is the brand; null when the brand is given */
  brandColumn: int32 | null;
}

/**
 * A column of the consent the Workspace collected: a row whose cell is one of the listed values
 * records an imported Confirmation of its Destination on the Channel, which lifts no refusal; any
 * other value records nothing
 */
model ContactImportConsentColumn {
  kind: "consent";
  channel: ContactImportConsentChannel;

  /** The values that mean consent, matched ignoring case and the spaces around them */
  @minItems(1)
  @maxItems(50)
  values: string[];

  evidence: ContactImportConsentEvidence;
}

/**
 * A column of Engagement tracking consent (ADR 0120): a row whose cell is one of the granted values
 * records a grant at its Destination on the Channel, one of the denied values a refusal; any other
 * value records nothing
 */
model ContactImportEngagementTrackingColumn {
  kind: "engagementTracking";
  channel: ContactImportConsentChannel;

  /** The values that mean tracking is granted */
  @maxItems(50)
  granted: string[];

  /** The values that mean tracking is denied */
  @maxItems(50)
  denied: string[];
}

/**
 * A column of when the fact of a refusal, consent or Engagement tracking column occurred (ADR
 * 0154), so an older fact does not override a newer one: a time blank, unreadable or later than the
 * run is dropped and reported, and the fact recorded as occurring now
 */
model ContactImportTimeColumn {
  kind: "time";

  /** The refusal, consent or Engagement tracking column it times, counted from 0 */
  of: int32;

  /** How the cells write their dates; a date without an offset is read in the Workspace time zone */
  dateFormat: ContactImportDateFormat;
}

model ContactImportConsentEvidenceInput is ContactImportConsentEvidence;
model ContactImportRefusalValueInput is ContactImportRefusalValue;

/** A column of the old platform's refusals, as a request writes it */
model ContactImportRefusalColumnInput {
  kind: "refusal";
  channel: ContactImportRefusalChannel;

  /** The values that mean a refusal, each with what it records */
  @minItems(1)
  @maxItems(50)
  values: ContactImportRefusalValueInput[];
}

/** A column of the consent the Workspace collected, as a request writes it */
model ContactImportConsentColumnInput {
  kind: "consent";
  channel: ContactImportConsentChannel;

  /** The values that mean consent, matched ignoring case and the spaces around them */
  @minItems(1)
  @maxItems(50)
  values: string[];

  evidence: ContactImportConsentEvidenceInput;
}

model ContactImportEngagementTrackingColumnInput
  is ContactImportEngagementTrackingColumn;
model ContactImportTimeColumnInput is ContactImportTimeColumn;

model ContactImportKeyColumnInput is ContactImportKeyColumn;
model ContactImportAttributeColumnInput is ContactImportAttributeColumn;
model ContactImportCustomFieldColumnInput is ContactImportCustomFieldColumn;
model ContactImportTagsColumnInput is ContactImportTagsColumn;
model ContactImportIgnoreColumnInput is ContactImportIgnoreColumn;

/** What a column of the file is, by `kind` */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union ContactImportColumn {
  key: ContactImportKeyColumn,
  attribute: ContactImportAttributeColumn,
  customField: ContactImportCustomFieldColumn,
  tags: ContactImportTagsColumn,
  ignore: ContactImportIgnoreColumn,
  refusal: ContactImportRefusalColumn,
  consent: ContactImportConsentColumn,
  engagementTracking: ContactImportEngagementTrackingColumn,
  time: ContactImportTimeColumn,
}

/** What a column of the file is, by `kind`, as a request writes it */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union ContactImportColumnInput {
  key: ContactImportKeyColumnInput,
  attribute: ContactImportAttributeColumnInput,
  customField: ContactImportCustomFieldColumnInput,
  tags: ContactImportTagsColumnInput,
  ignore: ContactImportIgnoreColumnInput,
  refusal: ContactImportRefusalColumnInput,
  consent: ContactImportConsentColumnInput,
  engagementTracking: ContactImportEngagementTrackingColumnInput,
  time: ContactImportTimeColumnInput,
}

/** How an import reads its file: what each column is, and the choices that hold for every row */
model ContactImportMapping {
  /** One per column of the file, in the file's order */
  columns: ContactImportColumn[];

  /** The character the file's numbers write their decimals after */
  decimalSeparator: ContactImportDecimalSeparator;

  /** The country a phone written without its country code is read in, when its row names no country; null for none */
  defaultCountry: CountryCode | null;

  /** A Tag given to every row, created on first use; null for none */
  tag: Name | null;

  /** Leave a Contact the Workspace has as it is, writing only new ones */
  onlyAddNew: boolean;
}

/** How an import reads its file, as a request writes it */
model ContactImportMappingInput {
  /** One per column of the file, in the file's order: as many as the file has */
  @maxItems(200)
  columns: ContactImportColumnInput[];

  /** The character the file's numbers write their decimals after */
  decimalSeparator: ContactImportDecimalSeparator;

  /** The country a phone written without its country code is read in, when its row names no country; null for none */
  defaultCountry: CountryCode | null;

  /** A Tag given to every row, created on first use; null for none */
  tag: Name | null;

  /** Leave a Contact the Workspace has as it is, writing only new ones */
  onlyAddNew: boolean;
}

/** What an import's run has written so far */
model ContactImportCounts {
  /** Rows read so far */
  processed: int32;

  /** Contacts created */
  created: int32;

  /** Contacts the rows updated, or left as they were with only new ones added */
  updated: int32;

  /** Rows refused: no alias key, a cell count other than the header's, or a row the Contact intake refused */
  refused: int32;

  /** Values dropped from rows that were written, because their column could not read them */
  dropped: int32;

  /** Opt-outs the refusal columns recorded */
  optedOut: int32;

  /** Suppressions the refusal columns recorded */
  suppressed: int32;
}

/** A CSV import of Contacts (ADR 0138): its file's facts, its mapping and how far its run is */
model SiteContactImport {
  /** Unique identifier */
  id: EntityId;

  state: ContactImportState;

  /** The character the file's cells are separated by */
  delimiter: ContactImportDelimiter;

  /** The file's header, a name per column; a blank name stays blank */
  header: string[];

  /** The file's rows below its header */
  rows: int32;

  /** Rows whose number of cells differs from the header's, which the import refuses */
  malformedRows: int32;

  /** The saved mapping, or before one is saved the proposal read from the header and the first rows: a header that names a key, a core attribute or a Custom field is mapped to it, a date column gets the format its sample reads in when only one fits */
  mapping: ContactImportMapping;

  counts: ContactImportCounts;

  /** Rows of the refused rows' report, refused rows and rows with dropped values: 0 when there is nothing to download */
  reportRows: int32;

  /** Upload timestamp */
  createdAt: Timestamp;

  /** When the run began; null before it */
  startedAt: Timestamp | null;

  /** When the run ended; null before it */
  endedAt: Timestamp | null;
}

/** Why a value or a row of an import is not written */
enum ContactImportIssueReason {
  /** The row names no alias key */
  noKey,

  /** The row has another number of cells than the header */
  cellCount,

  /** The Contact intake refused the row; the message says why */
  refused,

  /** The text is longer than the attribute takes */
  tooLong,

  /** Not a number with the file's decimal separator */
  notNumber,

  /** Not one of true/false, yes/no, 1/0, t/f, y/n */
  notBoolean,

  /** Not a date in the column's format */
  notDate,

  /** Not a phone number, or one without a country code where none is known */
  notPhone,

  /** Not an email address */
  notEmail,

  /** Not a time zone name */
  notTimeZone,

  /** Not a two-letter country code */
  notCountry,

  /** Not a language tag */
  notLocale,

  /** Not an https address of an image */
  notImageUrl,

  /** Not a Tag name */
  notTag,

  /** A time column's cell is blank on a row whose fact it times: the fact is recorded as occurring now */
  noTime,

  /** A time later than the run: the fact is recorded as occurring now */
  timeInFuture,

  /** A refusal, consent or tracking value on a row with no Destination on its Channel: nothing is recorded for it */
  noDestination,

  /** A consent value on a row whose evidence cell is blank or too long: no consent is recorded for it */
  noEvidence,

  /** A refusal the row names could not be recorded; the message says why */
  refusalNotRecorded,

  /** The row's alias keys name two Contacts: it was written to the one the keys' rank picks (subject ID, then email, then phone), without the keys of the other */
  keysNameTwoContacts,
}

/** A value or a row of an import that is not written, or a row written with a warning, and why */
model ContactImportIssue {
  /** The column, counted from 0; null for an issue of the whole row */
  column: int32 | null;

  reason: ContactImportIssueReason;

  /** Why, in words */
  message: string;
}

/**
 * A trait of a previewed row worth a look; the row is written all the same. `disposable_email` and
 * `role_email` are ADR 0117's verdicts on its address.
 */
enum ContactImportWarning {
  disposable_email,
  role_email,
}

/** A row of the file as the import would write it */
model ContactImportPreviewRow {
  /** The row's number below the header, from 1 */
  row: int32;

  /** The row's cells as the file writes them */
  cells: string[];

  /** Why the row would be refused; null when it would be written */
  refusal: ContactImportIssue | null;

  /** The values the row would drop */
  dropped: ContactImportIssue[];

  warnings: ContactImportWarning[];
}

/** What an import would do with its saved mapping (ADR 0138): its first rows, and counts over the whole file */
model SiteContactImportPreview {
  /** The first rows of the file */
  @maxItems(20)
  rows: ContactImportPreviewRow[];

  /** Rows of the file below its header */
  totalRows: int32;

  /** Rows that name no alias key, which the import refuses */
  rowsWithoutKey: int32;

  /** Rows whose number of cells differs from the header's, which the import refuses */
  malformedRows: int32;

  /** Distinct Tags the rows would add, the Tag for every row included */
  distinctTags: int32;

  /** Opt-outs the refusal columns would record over the whole file: the rows with a Destination on the column's Channel whose value means an Unsubscribe (on Telegram, the rows that name a key). An imported Unsubscribe cannot be undone: only the person lifts it */
  optOuts: int32;

  /** Suppressions the refusal columns would record over the whole file, counted as the opt-outs are */
  suppressions: int32;
}

/** A link a browser follows once, in the session that asked for it, within a minute */
model DownloadLink {
  /** Where the browser goes to download: a path of this Instance */
  url: string;

  /** When the link stops working */
  expiresAt: Timestamp;
}

@route("/workspaces/{workspaceId}/contact-imports")
@tag("Site Contact Imports")
namespace SiteContactImports {
  /**
   * Upload a CSV file of Contacts to import (ADR 0138): UTF-8 (a byte order mark allowed), its
   * cells separated by commas, semicolons or tabs, a header row first. It is refused whole when it
   * is not UTF-8, is larger than the Instance's import limit (413), has no header, has more than 200
   * columns or cannot be read as CSV. The answer proposes a mapping from the header.
   */
  @post
  @requiresPermissions(#[Permission.contactsImport])
  op upload(
    @path workspaceId: WorkspaceId,
    @header contentType: "text/csv",
    @body body: bytes,
  ): SiteContactImport | Problem;

  /** Read an import: its state, its mapping and how far its run is */
  @get
  @route("/{importId}")
  @requiresPermissions(#[Permission.contactsImport])
  op get(@path workspaceId: WorkspaceId, @path importId: EntityId):
    | SiteContactImport
    | Problem;

  /**
   * Save how the import reads its file, one entry per column; it is then `ready`. Refused before it
   * runs only: a running or ended import keeps its mapping (409). A refusal column recording a
   * Suppression needs `suppressions:manage` (403).
   */
  @put
  @route("/{importId}/mapping")
  @requiresPermissions(#[Permission.contactsImport])
  op saveMapping(
    @path workspaceId: WorkspaceId,
    @path importId: EntityId,
    @body body: ContactImportMappingInput,
  ): SiteContactImport | Problem;

  /** Preview the saved mapping over the file, writing nothing */
  @get
  @route("/{importId}/preview")
  @requiresPermissions(#[Permission.contactsImport])
  op preview(@path workspaceId: WorkspaceId, @path importId: EntityId):
    | SiteContactImportPreview
    | Problem;

  /**
   * Run a `ready` import: its job writes the rows through the Contact intake, acting as the Member,
   * and starts nothing (its Events are history). A date column without a format is refused (422),
   * and a refusal column recording a Suppression without `suppressions:manage` (403).
   */
  @post
  @route("/{importId}/run")
  @requiresPermissions(#[Permission.contactsImport])
  op run(@path workspaceId: WorkspaceId, @path importId: EntityId):
    | SiteContactImport
    | Problem;

  /** A link to the refused rows' report, good once, for this session, for a minute */
  @post
  @route("/{importId}/report-link")
  @requiresPermissions(#[Permission.contactsImport])
  op reportLink(@path workspaceId: WorkspaceId, @path importId: EntityId):
    | DownloadLink
    | Problem;

  /**
   * Download the refused rows' report the link names: the file's header and each reported row's
   * original cells, with a column saying why. It is deleted once downloaded whole; the link works
   * once, in the session that asked for it, within a minute (400 otherwise).
   */
  @get
  @route("/{importId}/report/{token}")
  @requiresPermissions(#[Permission.contactsImport])
  op report(
    @path workspaceId: WorkspaceId,
    @path importId: EntityId,
    @path token: string,
  ):
    | {
        @header contentType: "text/csv";
        @header("Content-Disposition") contentDisposition: string;
        @body body: bytes;
      }
    | Problem;
}
