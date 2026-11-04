namespace Sphericon;

/** How a Confirmation was obtained, strongest first: a double opt-in (the person clicked the link), a keyword (the person texted START), imported (consent an API client or an import collected itself, with its evidence), grandfathered (a prior basis asserted when confirmed opt-in was turned on). A Confirmation is replaced only by a stronger one. */
enum ConfirmationProvenance {
  double_opt_in,
  keyword,
  imported,
  grandfathered,
}

/** The evidence of consent collected outside sphericon (ADR 0077), kept in the immutable `sms.confirmed` Event to answer a proof request */
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

/** How a person stopped on a Channel other than email (ADR 0077): a STOP keyword or `/stop` (keyword), a message's unsubscribe button (button) or link (link), a blocked bot (block), a send the provider refused because the person had stopped at the provider (refused_send), or an opt-out recorded by hand in the app or through the API, which reached the Workspace by another route (manual) */
enum OptOutVia {
  keyword,
  button,
  block,
  link,
  refused_send,
  manual,
}

/** The SMS Confirmation of a contact's phone: when and how the number consented, with the evidence it carries */
model SmsConfirmation {
  /** How the consent was obtained */
  provenance: ConfirmationProvenance;

  /** The consent wording the person was shown; absent when the confirmation carries no evidence */
  wording?: string | null;

  /** Where the consent was collected; absent when the confirmation carries no evidence */
  source?: string | null;

  /** The brand the person agreed to hear from; absent when the confirmation carries no evidence */
  brand?: string | null;

  /** When the consent was recorded */
  confirmedAt: Timestamp;
}

/** The opt-out of a contact's phone from every SMS of the Workspace: when, and how the person stopped */
model SmsOptOut {
  /** How the person stopped: a STOP texted to the Workspace (keyword), a text's unsubscribe link (link), a send Twilio refused because the number had texted STOP to the sender there (refused_send), or recorded by hand (manual) */
  via: OptOutVia;

  /** When the phone opted out */
  optedOutAt: Timestamp;
}

/** Whether a contact's phone may be texted (ADR 0077): its SMS Confirmation and its SMS opt-out, both keyed by the number */
model SmsConsent {
  /** The contact's phone in E.164; absent when it has none, and then nothing else is */
  phone?: string | null;

  /** The phone's SMS Confirmation; absent when the number never consented, and a text does not go to it */
  confirmation?: SmsConfirmation | null;

  /** The phone's opt-out of every SMS of the Workspace; absent while it has none */
  optOut?: SmsOptOut | null;
}
