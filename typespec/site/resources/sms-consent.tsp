using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** The SMS Confirmation of a contact's phone: when and how the number consented, with the evidence it carries */
model SiteSmsConfirmation {
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
model SiteSmsOptOut {
  /** How the person stopped: a STOP texted to the Workspace (keyword), a text's unsubscribe link (link), a send Twilio refused because the number had texted STOP to the sender there (refused_send), or recorded by hand (manual) */
  via: OptOutVia;

  /** When the phone opted out */
  optedOutAt: Timestamp;
}

/** Whether a contact's phone may be texted (ADR 0077): its SMS Confirmation and its SMS opt-out, both keyed by the number */
model SiteSmsConsent {
  /** The contact's phone in E.164; absent when it has none, and then nothing else is */
  phone?: string | null;

  /** The phone's SMS Confirmation; absent when the number never consented, and a text does not go to it */
  confirmation?: SiteSmsConfirmation | null;

  /** The phone's opt-out of every SMS of the Workspace; absent while it has none */
  optOut?: SiteSmsOptOut | null;
}

@route("/workspaces/{workspaceId}/contacts/{contactId}")
@tag("Site SMS consent")
namespace SiteSmsConsents {
  /** Read the SMS Confirmation and opt-out of a contact's phone */
  @route("/sms-consent")
  @get
  @requiresPermissions(#[])
  op get(@path workspaceId: WorkspaceId, @path contactId: EntityId):
    | SiteSmsConsent
    | BadRequestProblem
    | NotFoundProblem;

  /**
   * Record consent the contact's phone gave outside sphericon, as an `imported` SMS Confirmation
   * with its evidence (the same as `POST /api/confirmations`). A stronger Confirmation already on
   * the number stays. It does not lift an opt-out. 422 when the contact has no phone.
   */
  @route("/sms-confirmations")
  @post
  @requiresPermissions(#[])
  op confirm(
    @path workspaceId: WorkspaceId,
    @path contactId: EntityId,
    @body body: ConsentEvidence,
  ):
    | SiteSmsConsent
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /**
   * Record an SMS opt-out that reached you by another route (an email, a call): the contact's
   * number gets no more texts from the Workspace. Idempotent. 422 when the contact has no phone.
   */
  @route("/sms-opt-outs")
  @post
  @requiresPermissions(#[])
  op optOut(@path workspaceId: WorkspaceId, @path contactId: EntityId):
    | SiteSmsConsent
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;
}
