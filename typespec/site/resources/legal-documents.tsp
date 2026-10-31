using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** A Sign-up item in effect, with the label of its current Legal document version: what the
 * sign-up form asks (ADR 0062) */
model SiteSignupItem {
  key: SignupItemKey;

  /** What the form shows */
  title: string;

  kind: SignupItemKind;

  /** The label of the current version, the one a person accepts */
  version: LegalDocumentLabel;
}

/** A person's answer to one Sign-up item the form showed (ADR 0062). An item named that is not in
 * effect, a version that is not its current one, or an item named twice refuses the form. */
model SiteSignupItemAnswer {
  key: SignupItemKey;

  /** The label of the version the form showed */
  version: LegalDocumentLabel;

  /** Whether the box was ticked; absent is unticked. A notice has no box, and its answer only
   * names the version shown. */
  accepted?: boolean;
}

/** The Sign-up items in effect, in the order of the sign-up form */
model SiteSignupItemList {
  items: SiteSignupItem[];
}

/** The current version of a Sign-up item's Legal document */
model SiteLegalDocument {
  key: SignupItemKey;

  /** The item's title */
  title: string;

  /** The label of the version */
  version: LegalDocumentLabel;

  markdown: LegalDocumentMarkdown;

  /** When the version was published, and took effect */
  publishedAt: Timestamp;
}

@tag("Site Legal Documents")
namespace SiteLegalDocuments {
  /** The Sign-up items in effect: those not archived that have a published version, with that
   * version's label. Public; empty when the Instance has none. Answered while an acceptance
   * is pending (ADR 0062). */
  @useAuth(NoAuth)
  @allowedWhileAcceptancePending
  @get
  @route("/signup-items")
  op signupItems(): {
    @statusCode statusCode: 200;
    @body body: SiteSignupItemList;
  };

  /** The current version of a Sign-up item's Legal document, for its public page. Public. 404
   * for an unknown or archived item and for one with no published version. Answered while an
   * acceptance is pending, for the screen that shows the new texts (ADR 0062). */
  @useAuth(NoAuth)
  @allowedWhileAcceptancePending
  @get
  @route("/legal-documents/{key}")
  op get(@path key: SignupItemKey): SiteLegalDocument | NotFoundProblem;
}
