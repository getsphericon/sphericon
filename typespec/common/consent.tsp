namespace Sphericon;

/** How a Confirmation was obtained, strongest first: a double opt-in (the person clicked the link), a keyword (the person texted START), imported (consent an API client or an import collected itself, with its evidence), grandfathered (a prior basis asserted when confirmed opt-in was turned on). A Confirmation is replaced only by a stronger one. */
enum ConfirmationProvenance {
  double_opt_in,
  keyword,
  imported,
  grandfathered,
}

/** The evidence of consent collected outside sphericon (ADR 0077), kept in the immutable `marketing.confirmed` Event to answer a proof request */
model ConsentEvidence {
  /** The consent wording the person was shown and agreed to */
  @minLength(1)
  @maxLength(2000)
  wording: string;

  /** Where the consent was collected: a form, a page, a checkout, a verification service */
  @minLength(1)
  @maxLength(500)
  source: string;

  /** The brand the person agreed to hear from */
  @minLength(1)
  @maxLength(255)
  brand: string;
}
