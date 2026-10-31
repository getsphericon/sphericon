using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** Outcome of a transactional send */
enum SiteTransactionalEmailStatus {
  /** Claimed but not yet completed (in flight, or left behind by a crash) */
  pending,

  /** Accepted by the email provider */
  sent,

  /** Skipped: the destination was on the workspace suppression list */
  suppressed,

  /** The provider call failed */
  failed,
}

/** The channel a transactional send went out on (email today; sms reserved) */
enum SiteTransactionalEmailChannel {
  email,
}

/** One transactional send: the durable, read-only trace of an `/api/emails` call */
@resource("transactional-emails")
@segment("transactional-emails")
@parentResource(SiteWorkspaceResource)
model SiteTransactionalEmailResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** The channel this send went out on */
  channel: SiteTransactionalEmailChannel;

  /** Normalized (lower-cased) destination address */
  destination: string;

  /** The id of the Template rendered at send time (bound by reference) */
  templateId: EntityId;

  /** The contact this destination resolved to, when one existed */
  contactId?: EntityId | null;

  /** Outcome of the send */
  status: SiteTransactionalEmailStatus;

  /** Provider error message, present when status is `failed` */
  error?: string | null;

  /** True when the send went through a Test inbox: captured, not delivered */
  captured: boolean;

  /** Creation timestamp */
  createdAt: Timestamp;
}

// Read-only send trace: list only. Nested via @parentResource + @autoRoute.
@tag("Site Transactional Emails")
namespace SiteTransactionalEmails {
  /** List transactional sends, most recent first */
  @listsResource(SiteTransactionalEmailResource)
  @get
  @autoRoute
  @requiresPermissions(#[])
  op list(
    ...ResourceCollectionParameters<SiteTransactionalEmailResource>,
    ...PageQuery,
  ):
    | Page<SiteTransactionalEmailResource>
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;
}
