using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** What a Needs attention item is about */
enum SiteAttentionKind {
  /** A re-check found a verified Sending domain's DKIM record gone: its mail is held */
  sendingDomainVerificationLost,

  /** The provider has stopped the Integration's account: every send through it is held (ADR 0067) */
  integrationProviderStopped,

  /** A Broadcast is held by a check of its own: no Integration can send it, its Sending domain is
   * not verified, or its provider stopped the account. `holdReason` says which. */
  broadcastHeld,

  /** A Sending domain is in the abuse detector's warning tier (ADR 0026); only where it runs */
  deliverabilityWarning,

  /** The Workspace sent its Send cap over the last 24 hours: sending is deferred until it refills */
  sendCapSpent,

  /** An Integration sent its Provider limit over the last 24 hours */
  providerLimitSpent,

  /** A Broadcast scheduled within the next 24 hours that the Workspace's Send cap will not let out
   * in full when it starts, together with the Broadcasts scheduled before it: it is deferred (ADR 0067)
   * and finishes at `estimatedCompletionAt`. `count` is its estimated audience. */
  scheduledBroadcastOverSendCap,

  /** The provider has the Integration's account under review; it still sends */
  integrationProviderProbation,

  /** The provider keeps the Integration's account in its sandbox: it sends only to verified addresses */
  integrationProviderSandbox,

  /** Runs of an Automation failed in the last 7 days; `count` says how many */
  enrollmentsFailed,

  /** A Broadcast failed in the last 7 days */
  broadcastFailed,

  /** A Webhook endpoint disabled itself after failing deliveries */
  webhookDisabled,

  /** A verified Sending domain whose last DMARC lookup found no published record: it is not
   * bulk-ready (ADR 0012), and Gmail and Yahoo reject or spam-folder bulk mail from it */
  sendingDomainNotBulkReady,
}

/** How urgent an item is */
enum SiteAttentionSeverity {
  /** Something does not send now */
  blocked,

  /** It sends, slower or at risk */
  degraded,

  /** Something failed and was not retried, or a setup that bulk mail needs is missing */
  notice,
}

/** The object a Needs attention item is about */
model SiteAttentionSubject {
  /** Its id: a Sending domain, an Integration, a Broadcast, an Automation or a Webhook endpoint, by `kind` */
  id: EntityId;

  /** Its name: the domain, the label, the name, or the endpoint's URL */
  name: string;
}

/** One thing in the Workspace that needs a member's action now */
model SiteAttentionItem {
  kind: SiteAttentionKind;
  severity: SiteAttentionSeverity;

  /** The object the item is about; absent for the Workspace's own Send cap and a deliverability warning */
  subject?: SiteAttentionSubject | null;

  /** How many the item stands for: failed runs of an Automation, the estimated audience of a
   * `scheduledBroadcastOverSendCap` Broadcast, 1 for every other kind */
  count: int32;

  /** When the problem began, when that is known */
  since?: Timestamp | null;

  /** Why a held Broadcast is held, set on a `broadcastHeld` item only. Only a reason of the Broadcast's
   * own source makes an item: a Workspace-level hold is the layout's banner */
  holdReason?: BroadcastHoldReason | null;

  /** The rate behind a deliverability warning */
  abuse?: SiteAbuseWarning | null;

  /** When the Send cap is expected to have let the Broadcast out, set on a
   * `scheduledBroadcastOverSendCap` item only: the same estimate as the Broadcast's progress once
   * it is sending */
  estimatedCompletionAt?: Timestamp | null;
}

/** What in the Workspace needs a member's action now, most urgent first and, within a severity,
 * the oldest first. Every item is true at the moment it is read; a Workspace suspension and a
 * Staff hold are not items, the Workspace itself carries them. */
model SiteAttentionList {
  items: SiteAttentionItem[];
}

@route("/workspaces/{workspaceId}/attention")
@tag("Site Attention")
namespace SiteAttention {
  /** What needs attention in the Workspace. Open to every member. */
  @get
  @requiresPermissions(#[])
  op get(@path workspaceId: WorkspaceId):
    | SiteAttentionList
    | BadRequestProblem
    | NotFoundProblem;
}
