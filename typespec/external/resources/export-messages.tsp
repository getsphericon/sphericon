using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/**
 * One row of the Outbound messages stream: the delivery record of one send, whichever
 * surface asked for it. It carries provenance and outcome only, never a subject or a body.
 * A row is delivered again whenever it changes (a status change, an Erasure clearing the
 * destination and the Contact).
 */
model ExportMessageRow {
  /** Unique identifier */
  id: EntityId;

  /** Which surface asked for the send: `broadcast`, `automation` or `transactional` */
  kind: string;

  /** Delivery channel */
  channel: string;

  /** Lower-cased destination address; absent once an Erasure anonymized the row */
  destination: string | null;

  /** `pending` (claimed, provider not yet answered), `sent`, `skipped` or `failed` */
  status: string;

  /** Why the message was skipped, or the failure text */
  reason: string | null;

  /** The Contact the destination resolved to; absent for a stranger or after an Erasure */
  contact_id: EntityId | null;

  /** The Broadcast the message belongs to */
  broadcast_id: EntityId | null;

  /** The Automation the message belongs to */
  automation_id: EntityId | null;

  /** The Automation run that sent the message */
  automation_run_id: EntityId | null;

  /** The step of the Automation that sent the message */
  automation_step: int32 | null;

  /** The Integration the message went out through */
  integration_id: EntityId | null;

  /** The Template a transactional send referenced */
  template_id: EntityId | null;

  /** The unsubscribe Sending source; absent for a transactional send */
  sending_source: string | null;

  /** Domain of the effective From address */
  sending_domain: string | null;

  /** The provider's id for the accepted message */
  provider_message_id: string | null;

  /** When the send was claimed, before the provider was called */
  claimed_at: Timestamp;

  /** When the provider accepted the message */
  sent_at: Timestamp | null;

  /** When the provider first reported the message delivered */
  delivered_at: Timestamp | null;

  /** When the provider first reported the message bounced */
  bounced_at: Timestamp | null;

  /** When the provider first reported a spam complaint about the message */
  complained_at: Timestamp | null;

  /** True when the message went through a Test inbox: captured, not delivered */
  captured: boolean;

  /** Creation timestamp */
  created_at: Timestamp;

  /** Last change timestamp: the cursor column and the version of the row */
  updated_at: Timestamp;
}

namespace Export {
  /**
   * Read the Workspace's Outbound messages, oldest change first (Enterprise; needs the
   * `export:read` scope). Each line of the body is an `ExportMessageRow`; every key is
   * always present.
   */
  @get
  @route("/messages")
  @requiresScopes(#[ApiTokenScope.exportRead])
  op messages(...ExportPageQuery):
    | ExportPage
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | NotFoundProblem;
}
