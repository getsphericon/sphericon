using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** Contact resource used by the site UI */
@resource("contacts")
@segment("contacts")
@parentResource(SiteWorkspaceResource)
model SiteContactResource {
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
  firstName?: string | null;

  /** Last name */
  lastName?: string | null;

  /** IANA time zone identifier */
  timeZone?: TimeZoneName | null;

  /** ISO 3166-1 alpha-2 country code */
  country?: CountryCode | null;

  /** Typed custom field values keyed by the field's machine key */
  customFields?: JsonObject | null;

  /** The labels the contact carries, ordered by name */
  tags: string[];

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/**
 * Site request body for creating a contact: the attributes /api takes, and its tags. The app's
 * contact form sets the tags in the same save; /api tags a contact through its own operations
 * (`tags_apply`, `tags_remove`), which is why its body has no tags.
 */
model SiteCreateContactInput {
  ...CreateContactInput;

  /** Tags to give the contact, created on first use */
  tags?: string[];
}

/**
 * Site request body for updating a contact: the attributes /api takes, and its tag set. The
 * app's contact form sets the tags in the same save; /api tags a contact through its own
 * operations (`tags_apply`, `tags_remove`), which is why its body has no tags.
 */
model SiteUpdateContactInput {
  ...UpdateContactInput;

  /** The contact's complete tag set when given: tags not listed are removed from it, an absent key keeps them */
  tags?: string[];
}

/** Attachment download of everything held about one contact (subject access, GDPR Art. 15/20): the ContactExportDocument JSON, streamed */
model SiteContactExportDownload {
  @header contentType: "application/octet-stream";
  @header("Content-Disposition") contentDisposition: string;
  @body body: bytes;
}

/** The same download described by its document type, for typed clients */
model SiteContactExportTypedDownload {
  @header contentType: "application/json";
  @header("Content-Disposition") contentDisposition: string;
  @body body: ContactExportDocument;
}

/** What a Workspace holds for one email address, as counts only (a Privacy request lookup). Nothing held is all zeros and `contactHeld: false`. */
model SiteContactHoldings {
  /** Whether a Contact holds the address */
  contactHeld: boolean;

  /** Events of the Contact holding the address, or carrying the address */
  events: int32;

  /** Delivery records (outbound messages) to the address */
  deliveryRecords: int32;

  /** Unsubscribes on the address */
  unsubscribes: int32;

  /** Suppressions on the address */
  suppressions: int32;

  /** Captured messages (Test inbox) sent to the address; a count, never their content */
  capturedMessages: int32;
}

/** Narrows the Contacts list */
model SiteContactsListFilter {
  /** Keeps the contacts whose email address contains this text, ignoring case */
  @query email?: string;
}

@tag("Site Contacts")
interface SiteContacts
  extends WorkspaceCrudUnique<
      SiteContactResource,
      SiteCreateContactInput,
      SiteUpdateContactInput,
      ForbiddenProblem,
      SiteContactsListFilter
    > {
  /**
   * Export everything held about one contact as a streamed JSON download.
   * Identify the contact by exactly one of `id` or `email`. Owners and admins only.
   */
  @collectionAction(SiteContactResource, "export")
  @autoRoute
  @requiresPermissions(#[Permission.contactsExport])
  export(
    ...ResourceCollectionParameters<SiteContactResource>,

    /** The contact's id */
    @query id?: EntityId,

    /** The contact's email address */
    @query email?: EmailAddress,
  ):
    | SiteContactExportDownload
    | SiteContactExportTypedDownload
    | BadRequestProblem
    | UnprocessableEntityProblem
    | ForbiddenProblem
    | NotFoundProblem;

  /**
   * Look up what the Workspace holds for one email address (a Privacy request):
   * counts only. The address is trimmed and lowercased; an invalid one is refused (422).
   * An address nothing holds answers with zeros, not 404. Owner or admin only.
   */
  @get
  @collectionAction(SiteContactResource, "lookup")
  @autoRoute
  @requiresPermissions(#[Permission.contactsErase])
  lookup(
    ...ResourceCollectionParameters<SiteContactResource>,

    /** The email address, as typed: trimmed and validated by the erasure module, not by the format check */
    @maxLength(320) @query email: string,
  ):
    | SiteContactHoldings
    | BadRequestProblem
    | UnprocessableEntityProblem
    | ForbiddenProblem
    | NotFoundProblem;

  /**
   * Erase by email address (GDPR Art. 17, a Privacy request), the same Erasure as
   * deleting a contact: personal data is removed, delivery records to the address
   * are anonymized (also when no contact holds it) and opt-outs survive. Answers
   * 404 when neither a contact nor delivery records hold the address. Owner or
   * admin only.
   */
  @delete
  @collectionAction(SiteContactResource, "erase")
  @autoRoute
  @requiresPermissions(#[Permission.contactsErase])
  erase(
    ...ResourceCollectionParameters<SiteContactResource>,

    /** The email address to erase, as typed: trimmed and validated by the erasure module */
    @maxLength(320) @query email: string,
  ):
    | NoContentResponse
    | BadRequestProblem
    | UnprocessableEntityProblem
    | ForbiddenProblem
    | NotFoundProblem;
}

@@doc(
  SiteContacts.delete,
  "Erase a contact (GDPR Art. 17): its personal data is removed, delivery records are anonymized and its opt-outs survive. Owner or admin only."
);

@@requiresPermissions(SiteContacts.create, #[]);
@@requiresPermissions(SiteContacts.delete, #[Permission.contactsErase]);
@@requiresPermissions(SiteContacts.get, #[]);
@@requiresPermissions(SiteContacts.list, #[]);
@@requiresPermissions(SiteContacts.update, #[]);
