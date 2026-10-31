using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** Contact resource */
@resource("contacts")
@segment("contacts")
model ContactResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** The customer's own user id — an alias key. May be absent. */
  subjectId?: string | null;

  /** Email address — an alias key. May be absent for anonymous contacts. */
  email?: EmailAddress | null;

  /** Phone number — an alias key. May be absent. */
  phone?: string | null;

  /** First name */
  @untrusted
  firstName?: string | null;

  /** Last name */
  @untrusted
  lastName?: string | null;

  /** IANA time zone identifier */
  timeZone?: TimeZoneName | null;

  /** Typed custom field values keyed by the field's machine key */
  @untrusted
  customFields?: JsonObject | null;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Attachment download of everything held about one contact (subject access, GDPR Art. 15/20): the ContactExportDocument JSON, streamed */
model ContactExportDownload {
  @header contentType: "application/octet-stream";
  @header("Content-Disposition") contentDisposition: string;
  @body body: bytes;
}

/** The same download described by its document type, for typed clients */
model ContactExportTypedDownload {
  @header contentType: "application/json";
  @header("Content-Disposition") contentDisposition: string;
  @body body: ContactExportDocument;
}

@tag("Contacts")
interface Contacts
  extends ExternalCrudUnique<
      ContactResource,
      CreateContactInput,
      UpdateContactInput,
      ApiTokenScope.contactsRead,
      ApiTokenScope.contactsWrite,
      ApiTokenScope.contactsErase
    > {
  /**
   * Export everything held about one contact as a streamed JSON download: the
   * contact, its custom fields, tags, visitors, all events, its opt-outs
   * (unsubscribes, suppressions, confirmations) and delivery metadata. Rendered
   * message bodies are not included. Identify the contact by exactly one of
   * `id` or `email`.
   */
  // The contacts:export scope needs the contacts:export Permission to be granted, and a
  // Connection's User holds it on every request (ADR 0061); as `email` finds a Contact by
  // address, the User also needs contacts:erase, as on /site.
  @collectionAction(ContactResource, "export")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.contactsExport])
  @requiresPermissions(#[Permission.contactsErase])
  export(
    ...ResourceCollectionParameters<ContactResource>,

    /** The contact's id */
    @query id?: EntityId,

    /** The contact's email address */
    @query email?: EmailAddress,
  ):
    | ContactExportDownload
    | ContactExportTypedDownload
    | BadRequestProblem
    | UnprocessableEntityProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  /**
   * Erase by an identifier other than the contact id (GDPR Art. 17): by `email`
   * (also anonymizes delivery records to an address that never had a contact) or
   * by `visitorId` (an anonymous visitor and its events). Exactly one of the two.
   * The same rules as deleting by id apply. Requires the contacts:erase scope.
   */
  @collectionAction(ContactResource, "erase")
  @delete
  @autoRoute
  @requiresScopes(#[ApiTokenScope.contactsErase])
  eraseBy(
    ...ResourceCollectionParameters<ContactResource>,

    /** The email address to erase */
    @query email?: EmailAddress,

    /** The anonymous visitor id to erase */
    @query visitorId?: string,
  ):
    | NoContentResponse
    | BadRequestProblem
    | UnprocessableEntityProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;
}

/** One contact to upsert, matched by its alias keys (subject id, email, phone) */
model UpsertContactInput {
  /** The customer's own user id — an alias key */
  subjectId?: string | null;

  /** Email address — an alias key */
  email?: EmailAddress | null;

  /** Phone number — an alias key */
  phone?: string | null;

  /** First name — filled only when the contact has none */
  firstName?: string | null;

  /** Last name — filled only when the contact has none */
  lastName?: string | null;

  /** IANA time zone identifier — filled only when the contact has none */
  timeZone?: TimeZoneName | null;

  /** Typed custom field values keyed by the field's machine key, merged into the contact's */
  customFields?: JsonObject | null;
}

/** Request body for upserting contacts in batch */
model UpsertContactsInput {
  /** Contacts to upsert, at most 1000 */
  @minItems(1)
  @maxItems(1000)
  contacts: UpsertContactInput[];
}

/** Outcome of one item of a contacts batch */
enum ContactBatchStatus {
  created,
  updated,
  failed,
}

/** Per-item result of a contacts batch, in request order */
model ContactBatchItemResult {
  /** Zero-based position of the item in the request */
  index: int32;

  /** created or updated on success; failed when the item was rejected */
  status: ContactBatchStatus;

  /** The contact the item resolved to; absent when failed */
  contactId?: EntityId;

  /** Why the item failed; absent on success */
  error?: string;
}

/** Per-item results of a contacts batch */
model UpsertContactsResult {
  /** One result per request item, in request order */
  results: ContactBatchItemResult[];
}

@route("/contacts/batch")
@tag("Contacts")
namespace ContactsBatch {
  /** Upsert up to 1000 contacts by alias keys; a failing item does not affect the others */
  @post
  @requiresScopes(#[ApiTokenScope.contactsWrite])
  op upsert(@body body: UpsertContactsInput):
    | UpsertContactsResult
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | BadRequestProblem
    | UnprocessableEntityProblem;
}

@@doc(
  Contacts.delete,
  "Erase a contact (GDPR Art. 17): its personal data is removed, delivery records are anonymized and its opt-outs survive. Requires the contacts:erase scope."
);
