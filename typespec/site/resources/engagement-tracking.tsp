using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** An Engagement tracking consent a member records on the Contact card for one of the contact's Destinations (ADR 0120), as `manual` */
model SiteRecordEngagementTrackingInput {
  /** The Channel the consent is for. Only `email` is accepted yet */
  channel: EngagementTrackingChannel;

  /** The contact's Destination that consented or refused: its email address on `email` */
  @maxLength(320)
  destination: string;

  /** Whether the person agreed to have their opens and clicks tracked, or refused */
  state: EngagementTrackingState;
}

// A member records a refusal of tracking or a grant; a grant over a standing `denied` is refused,
// since only the person lifts a refusal (ADR 0121). There is deliberately no operation that lifts
// a `denied`: do not add one.
@route("/workspaces/{workspaceId}/contacts/{contactId}/engagement-tracking")
@tag("Site engagement tracking")
namespace SiteEngagementTracking {
  /**
   * Read whether each Destination of a contact (its email address) is tracked: the consent
   * recorded with who recorded it, or `default` where none is, with what the Workspace's default
   * means now
   */
  @get
  @requiresPermissions(#[])
  op get(@path workspaceId: WorkspaceId, @path contactId: EntityId):
    | ContactEngagementTracking
    | Problem;

  /**
   * Record a refusal or a grant of tracking the person made to you (by phone, by reply) for one of
   * the contact's Destinations, as `manual`, now. A `denied` always applies and is kept in the
   * immutable `email.engagement_tracking_denied` event; a `granted` is refused with a conflict over
   * a standing `denied`: only the person can lift a refusal. 422 when the Destination is not the
   * contact's.
   */
  @post
  @requiresPermissions(#[])
  op create(
    @path workspaceId: WorkspaceId,
    @path contactId: EntityId,
    @body body: SiteRecordEngagementTrackingInput,
  ): EngagementTrackingObservationResult | Problem;
}
