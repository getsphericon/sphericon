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
    | SmsConsent
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
    | SmsConsent
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;
}
