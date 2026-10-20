using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

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

  /** Typed custom field values keyed by the field's machine key */
  customFields?: Record<unknown> | null;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Site request body for creating a contact */
model SiteCreateContactInput {
  /** The customer's own user id — an alias key */
  subjectId?: string | null;

  /** Email address — optional; any alias key may be absent */
  email?: EmailAddress | null;

  /** Phone number — an alias key */
  phone?: string | null;

  /** First name */
  firstName?: string | null;

  /** Last name */
  lastName?: string | null;

  /** IANA time zone identifier */
  timeZone?: TimeZoneName | null;

  /** Typed custom field values keyed by the field's machine key */
  customFields?: Record<unknown> | null;
}

/** Site request body for updating a contact */
model SiteUpdateContactInput {
  /** The customer's own user id — an alias key */
  subjectId?: string | null;

  /** Email address — an alias key */
  email?: EmailAddress | null;

  /** Phone number — an alias key */
  phone?: string | null;

  /** First name */
  firstName?: string | null;

  /** Last name */
  lastName?: string | null;

  /** IANA time zone identifier */
  timeZone?: TimeZoneName | null;

  /** Typed custom field values keyed by the field's machine key */
  customFields?: Record<unknown> | null;
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
model SiteContactLookup {
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
}

@tag("Site Contacts")
interface SiteContacts
  extends WorkspaceCrudUnique<
      SiteContactResource,
      SiteCreateContactInput,
      SiteUpdateContactInput,
      ForbiddenProblem
    > {
  /**
   * Export everything held about one contact as a streamed JSON download.
   * Identify the contact by exactly one of `id` or `email`. Available to any
   * Workspace member (contact read access).
   */
  @collectionAction(SiteContactResource, "export")
  @autoRoute
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
    | NotFoundProblem;

  /**
   * Look up what the Workspace holds for one email address (a Privacy request):
   * counts only. The address is trimmed and lowercased; an invalid one is rejected.
   * An address nothing holds answers with zeros, not 404. Owner or admin only.
   */
  @get
  @collectionAction(SiteContactResource, "lookup")
  @autoRoute
  lookup(
    ...ResourceCollectionParameters<SiteContactResource>,

    /** The email address */
    @query email: EmailAddress,
  ): SiteContactLookup | BadRequestProblem | ForbiddenProblem | NotFoundProblem;
}

@@doc(
  SiteContacts.delete,
  "Erase a contact (GDPR Art. 17): its personal data is removed, delivery records are anonymized and its opt-outs survive. Owner or admin only."
);
