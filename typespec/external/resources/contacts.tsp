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

  /** Phone number in E.164 (`+14155550100`) — an alias key. May be absent. */
  phone?: string | null;

  /** First name */
  @untrusted
  firstName?: string | null;

  /** Last name */
  @untrusted
  lastName?: string | null;

  /** IANA time zone identifier */
  timeZone?: TimeZoneName | null;

  /** ISO 3166-1 alpha-2 country code */
  country?: CountryCode | null;

  /** BCP 47 language tag of the language the contact reads in */
  locale?: LocaleTag | null;

  /** Date of birth */
  birthday?: plainDate | null;

  /** City */
  @maxLength(255)
  city?: string | null;

  /** https URL of the contact's photo */
  avatar?: ImageUrl | null;

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
  extends ExternalCrud<
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
  ): ContactExportDownload | ContactExportTypedDownload | Problem;

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
  ): NoContentResponse | Problem;
}

/**
 * One contact to upsert, matched by its alias keys (subject id, email, phone). An alias key fills
 * only a contact that has none; every other attribute given replaces the contact's, and null clears
 * it.
 */
model UpsertContactInput {
  /** The customer's own user id — an alias key */
  subjectId?: string | null;

  /** Email address — an alias key */
  email?: EmailAddress | null;

  /** Phone number — an alias key, stored in E.164 (`+14155550100`). A number without a country code
   * is read with the contact's `country`; one that does not parse is refused on `phone` */
  phone?: string | null;

  /** First name */
  firstName?: string | null;

  /** Last name */
  lastName?: string | null;

  /** IANA time zone identifier */
  timeZone?: TimeZoneName | null;

  /** ISO 3166-1 alpha-2 country code */
  country?: CountryCode | null;

  /** BCP 47 language tag */
  locale?: LocaleTag | null;

  /** Date of birth */
  birthday?: plainDate | null;

  /** City */
  @maxLength(255)
  city?: string | null;

  /** https URL of the contact's photo */
  avatar?: ImageUrl | null;

  /** Typed custom field values keyed by the field's machine key, merged into the contact's. A new
   * key's first value sets the field's type: a boolean, a number, a date (an ISO string:
   * `2026-01-15T10:00:00Z`, `2026-01-15T10:00:00` or `2026-01-15`, the last two read in the
   * Workspace time zone) or text. Send a date without time as a `YYYY-MM-DD` string, not a
   * JavaScript `Date`: that serializes as UTC and lands on the previous day east of UTC */
  customFields?: JsonObject | null;

  /** The evidence of the SMS consent the contact's phone gave outside sphericon: records an
   * `imported` SMS confirmation for the phone, as `POST /confirmations` does. Without it a phone
   * stays unreachable on SMS. An item with it and no phone fails */
  smsConsent?: ConsentEvidence;
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

/**
 * A trait of an item's email address worth the importer's attention; the contact is stored all the
 * same. `disposable_email`: an address at a throwaway mailbox provider. `role_email`: an address of
 * a function rather than a person, such as info@ or sales@.
 */
enum ContactBatchWarning {
  disposable_email,
  role_email,
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

  /** Traits of the item's email address worth a look; absent when there are none or the item failed */
  warnings?: ContactBatchWarning[];
}

/** Per-item results of a contacts batch */
model UpsertContactsResult {
  /** One result per request item, in request order */
  results: ContactBatchItemResult[];
}

@route("/contacts/batch")
@tag("Contacts")
namespace ContactsBatch {
  /**
   * Upsert up to 1000 contacts by alias keys; a failing item does not affect the others. An item
   * that brings an email address new to the workspace fails when the address's domain cannot
   * receive mail: it does not exist, has no mail exchanger or publishes a null MX. A domain DNS
   * does not answer for is accepted.
   */
  @post
  @requiresScopes(#[ApiTokenScope.contactsWrite])
  op upsert(@body body: UpsertContactsInput): UpsertContactsResult | Problem;
}

@@doc(
  Contacts.delete,
  "Erase a contact (GDPR Art. 17): its personal data is removed, delivery records are anonymized and its opt-outs survive. Requires the contacts:erase scope."
);
