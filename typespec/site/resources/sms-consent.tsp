using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

@route("/workspaces/{workspaceId}/contacts/{contactId}")
@tag("Site SMS consent")
namespace SiteSmsConsents {
  /** Read the SMS Confirmation and opt-out of a contact's phone */
  @route("/sms-consent")
  @get
  @requiresPermissions(#[])
  op get(@path workspaceId: WorkspaceId, @path contactId: EntityId):
    | SmsConsent
    | Problem;

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
  ): SmsConsent | Problem;

  /**
   * Record an SMS opt-out that reached you by another route (an email, a call): the contact's
   * number gets no more texts from the Workspace. Idempotent. 422 when the contact has no phone.
   */
  @route("/sms-opt-outs")
  @post
  @requiresPermissions(#[])
  op optOut(@path workspaceId: WorkspaceId, @path contactId: EntityId):
    | SmsConsent
    | Problem;
}

@route("/workspaces/{workspaceId}/contacts/{contactId}/confirmation-requests")
@tag("Site Consent")
namespace SiteConfirmationRequests {
  /**
   * Send the contact a double opt-in confirmation email (ADR 0013), as the API's confirmation
   * request of a contact does: `confirmed` (200) when the address already
   * holds a Confirmation, 409 `confirmation_email_required` while the confirmation email is not set
   * up, 409 naming an opt-out of everything or a Suppression, 429 past 3 emails to the address in 24
   * hours.
   */
  @post
  @requiresPermissions(#[])
  op create(@path workspaceId: WorkspaceId, @path contactId: EntityId):
    | {
        @statusCode statusCode: 202;
        @body body: ConfirmationRequestResult;
      }
    | {
        @statusCode statusCode: 200;
        @body body: ConfirmationRequestResult;
      }
    | Problem;
}
