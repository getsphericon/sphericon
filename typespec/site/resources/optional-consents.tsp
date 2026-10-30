using TypeSpec.Http;

namespace Sphericon;

/** A User's current answer to an optional consent: the decision of their latest Consent record on
 * it (ADR 0062) */
enum SiteOptionalConsentDecision {
  /** Given, at sign-up or in account settings */
  accepted,

  /** Left unticked at sign-up */
  declined,

  /** Given once and taken back in account settings */
  withdrawn,
}

/** An optional consent in effect, with the User's current answer to it (ADR 0062) */
model SiteOptionalConsent {
  key: SignupItemKey;

  /** What the form shows */
  title: string;

  /** The label of the current version, the one a change answers */
  version: LegalDocumentLabel;

  /** The User's current answer; absent when they never answered, as a User who signed up before
   * the item existed */
  decision?: SiteOptionalConsentDecision;

  /** When the current answer was given; absent with no answer */
  decidedAt?: Timestamp;
}

/** The optional consents in effect, in the order of the sign-up form */
model SiteOptionalConsentList {
  items: SiteOptionalConsent[];
}

/** A change of the User's answer to an optional consent */
model SiteOptionalConsentChange {
  /** The label of the version the page showed; one that is no longer current refuses the change */
  version: LegalDocumentLabel;

  /** True gives the consent, false withdraws it */
  accepted: boolean;
}

@route("/me/optional-consents")
@tag("Site User")
namespace SiteOptionalConsents {
  /** The optional consents in effect (not archived, with a published version) and the User's
   * current answer to each. They belong to the User, so no Workspace is read. Empty when the
   * Instance has none. A required acceptance and a notice are not listed: they are not the User's
   * to change. */
  @get
  op list(): {
    @statusCode statusCode: 200;
    @body body: SiteOptionalConsentList;
  };

  /** Give or withdraw an optional consent. Giving it records `accepted` on the current version,
   * withdrawing a given one records `withdrawn`, both with the source account settings; a change
   * that leaves the answer as it is records nothing. 404 for an item not in effect. 422 for a
   * required acceptance or a notice, which cannot be changed here, and for a version that is not
   * the current one (field `version`). */
  @put
  @route("/{key}")
  op set(@path key: SignupItemKey, @body body: SiteOptionalConsentChange):
    | {
        @statusCode statusCode: 200;
        @body body: SiteOptionalConsent;
      }
    | NotFoundProblem
    | UnprocessableEntityProblem;
}
