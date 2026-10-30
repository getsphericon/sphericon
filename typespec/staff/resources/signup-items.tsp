using TypeSpec.Http;

namespace Sphericon;

/** A published Legal document version, as the console shows it */
model StaffLegalDocumentVersion {
  /** The label it was published with */
  label: LegalDocumentLabel;

  /** When it was published, and took effect */
  publishedAt: Timestamp;

  /** The Staff member who published it */
  publishedBy: StaffResource;
}

/** Every published version of an item's Legal document, the latest (the current one) first */
model StaffLegalDocumentVersionList {
  items: StaffLegalDocumentVersion[];
}

/** A Sign-up item of the Instance (ADR 0062) with its Legal document's draft and current
 * version */
model StaffSignupItemResource {
  /** Unique identifier */
  id: EntityId;

  /** The item's key: its Legal document's public page is served under it */
  key: SignupItemKey;

  /** What the sign-up form shows */
  title: SignupItemTitle;

  kind: SignupItemKind;

  /** Where the item stands in the sign-up form, smallest first */
  position: int32;

  /** Whether the item stands for consent to the operator's mailings */
  mailingsConsent: boolean;

  /** The draft of the next version, absent until one is written. A visitor never sees it. */
  draft?: LegalDocumentMarkdown;

  /** The version in effect, absent until one is published. Visitors see the item only once it
   * has one. */
  currentVersion?: StaffLegalDocumentVersion;

  /** When the item was archived, absent while it is in use. An archived item is no longer asked
   * and its public page is gone; its published versions stay. */
  archivedAt?: Timestamp;

  /** Creation timestamp */
  createdAt: Timestamp;
}

/** The Instance's Sign-up items, in the order of the sign-up form */
model StaffSignupItemList {
  items: StaffSignupItemResource[];
}

/** Add a Sign-up item. It goes after the existing ones. */
model StaffSignupItemInput {
  /** Unique across the Instance, and never changed */
  key: SignupItemKey;

  title: SignupItemTitle;

  /** Never changed */
  kind: SignupItemKind;

  /** Mark the item as consent to the operator's mailings: only an optional consent, and only
   * one item of the Instance */
  mailingsConsent?: boolean;
}

/** Change a Sign-up item */
model StaffSignupItemUpdate {
  /** A new title. It is not a new version of the Legal document: the form shows it at once. */
  title?: SignupItemTitle;

  /** Mark or unmark the item as consent to the operator's mailings: only an optional consent,
   * and only one item of the Instance */
  mailingsConsent?: boolean;
}

/** The new order of the sign-up form: the id of every Sign-up item, archived ones included, each
 * once */
model StaffSignupItemOrder {
  ids: EntityId[];
}

/** Save the draft of an item's Legal document */
model StaffLegalDocumentDraftInput {
  markdown: LegalDocumentMarkdown;
}

/** Publish the draft as a new version */
model StaffLegalDocumentPublishInput {
  /** Unique among the item's versions */
  label: LegalDocumentLabel;
}

@route("/signup-items")
@tag("StaffMember Signup Items")
namespace StaffSignupItems {
  /** The Instance's Sign-up items, archived ones included, in the order of the sign-up form */
  @get
  op list(): StaffSignupItemList | UnauthorizedProblem;

  /** Add a Sign-up item. A taken key, a mailings consent on an item that is not an optional
   * consent, and a second mailings consent are refused (422). */
  @post
  op create(@body body: StaffSignupItemInput):
    | {
        @statusCode statusCode: 201;
        @body body: StaffSignupItemResource;
      }
    | UnauthorizedProblem
    | UnprocessableEntityProblem;

  /** Put the Sign-up items in a new order, the order the sign-up form asks them in. A list that
   * is not every item's id exactly once is refused (422). */
  @put
  @route("/order")
  op reorder(@body body: StaffSignupItemOrder):
    | StaffSignupItemList
    | UnauthorizedProblem
    | UnprocessableEntityProblem;

  /** One Sign-up item */
  @get
  @route("/{id}")
  op get(@path id: EntityId):
    | StaffSignupItemResource
    | UnauthorizedProblem
    | NotFoundProblem;

  /** Change a Sign-up item. A blank title, a mailings consent on an item that is not an optional
   * consent or is archived, and a second mailings consent are refused (422). */
  @patch
  @route("/{id}")
  op update(@path id: EntityId, @body body: StaffSignupItemUpdate):
    | StaffSignupItemResource
    | UnauthorizedProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Archive the Sign-up item: it leaves the sign-up form and its public page at once, and its
   * published versions stay. Archiving an archived item changes nothing, and an archived item is never
   * restored. The item that stands for consent to the operator's mailings is refused (422) until
   * it is unmarked. */
  @post
  @route("/{id}/archive")
  op archive(@path id: EntityId):
    | StaffSignupItemResource
    | UnauthorizedProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Every published version of the item's Legal document with who published it and when, the
   * latest first */
  @get
  @route("/{id}/versions")
  op listVersions(@path id: EntityId):
    | StaffLegalDocumentVersionList
    | UnauthorizedProblem
    | NotFoundProblem;

  /** Save the draft of the item's Legal document. It is edited freely and changes nothing a
   * visitor sees until it is published. */
  @put
  @route("/{id}/draft")
  op saveDraft(@path id: EntityId, @body body: StaffLegalDocumentDraftInput):
    | StaffSignupItemResource
    | UnauthorizedProblem
    | NotFoundProblem;

  /** Publish the draft as a new version under a label: it takes effect at once, and is never
   * changed or removed afterwards. A label the item already has, and an item without a draft,
   * are refused (422). */
  @post
  @route("/{id}/versions")
  op publish(@path id: EntityId, @body body: StaffLegalDocumentPublishInput):
    | {
        @statusCode statusCode: 201;
        @body body: StaffSignupItemResource;
      }
    | UnauthorizedProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;
}
