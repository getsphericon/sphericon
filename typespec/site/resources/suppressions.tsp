using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** Why a destination is on the suppression (do-not-send) list. Unsubscribe is
 * not a suppression — it is a separate, per-source opt-out. */
enum SiteSuppressionReason {
  bounce,
  complaint,
  manual,
}

/** The channel a suppression applies to (email today; sms reserved) */
enum SiteSuppressionChannel {
  email,
}

/** A suppressed destination: the send path skips it on every surface */
@resource("suppressions")
@segment("suppressions")
@parentResource(SiteWorkspaceResource)
model SiteSuppressionResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** The channel this suppression applies to */
  channel: SiteSuppressionChannel;

  /** Normalized (lower-cased) channel-specific destination address */
  destination: string;

  /** Why the destination is suppressed */
  reason: SiteSuppressionReason;

  /** When the suppression was lifted; null while it is active. A lifted one no longer blocks sends. */
  liftedAt?: Timestamp | null;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Site request body for manually suppressing a destination */
model SiteCreateSuppressionInput {
  /** Channel (defaults to email when omitted) */
  channel?: SiteSuppressionChannel;

  /** The destination address to suppress */
  destination: string;
}

/** Query for the suppression list */
model SiteSuppressionListQuery {
  /** true lists the lifted suppressions instead of the active ones; the default hides them */
  @query lifted?: boolean;
}

// Bespoke (not WorkspaceCrud): no GET/PUT for a single suppression.
// Nested via @parentResource + @autoRoute.
@tag("Site Suppressions")
namespace SiteSuppressions {
  /** Manually suppress an address (reason = manual; idempotent per address). Owners and admins
   * only. */
  @createsResource(SiteSuppressionResource)
  @post
  @autoRoute
  @requiresPermissions(#[Permission.suppressionsManage])
  op create(
    ...ResourceCollectionParameters<SiteSuppressionResource>,
    @body body: SiteCreateSuppressionInput,
  ):
    | {
        @statusCode statusCode: 201;
        @body body: SiteSuppressionResource;
      }
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** List suppressed addresses (every member) */
  @listsResource(SiteSuppressionResource)
  @get
  @autoRoute
  @requiresPermissions(#[])
  op list(
    ...ResourceCollectionParameters<SiteSuppressionResource>,
    ...PageQuery,
    ...SiteSuppressionListQuery,
  ):
    | Page<SiteSuppressionResource>
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Lift a suppression: the row stays with its lift time and the address can receive mail again.
   * Lifting one that is already lifted is a 404. Owners and admins only. */
  @deletesResource(SiteSuppressionResource)
  @delete
  @autoRoute
  @requiresPermissions(#[Permission.suppressionsManage])
  op delete(...ResourceParameters<SiteSuppressionResource>):
    | NoContentResponse
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem;
}
