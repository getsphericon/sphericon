using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** A tracked event shown in the activity feed */
@resource("events")
@segment("events")
@parentResource(SiteWorkspaceResource)
model SiteEventResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** Subject (tracking profile / visitor) the event belongs to */
  subjectId: string;

  /** Email associated with the event, if known */
  email?: string | null;

  /** Event action / name */
  action: string;

  /** Arbitrary event properties */
  properties?: Record<unknown> | null;

  /** When the event happened: the time the source gave, or when it was published if it gave none */
  occurredAt: Timestamp;

  /** When the event was recorded */
  createdAt: Timestamp;
}

/** Distinct event actions seen in a workspace (for segment/filter pickers) */
model SiteEventActionsResult {
  actions: string[];
}

// Read-only activity feed + an actions metadata collection action.
// Nested via @parentResource + @autoRoute.
@tag("Site Events")
namespace SiteEvents {
  /** List events for the site UI (most recent first) */
  @listsResource(SiteEventResource)
  @get
  @autoRoute
  @requiresPermissions(#[])
  op list(
    ...ResourceCollectionParameters<SiteEventResource>,
    ...PageQuery,

    /** Filter by event action */
    @query action?: string,

    /** Filter by the Contact the event resolved to (the stable identity link) */
    @query contactId?: EntityId,

    /** Filter by the email associated with the event */
    @query email?: string,

    /** List the opens and clicks made by software (machine engagement) too; omitted, they are left out */
    @query includeMachine?: boolean,
  ): Page<SiteEventResource> | NotFoundProblem;

  /** Distinct event actions in the workspace, sorted — powers the segment builder */
  @collectionAction(SiteEventResource, "actions")
  @get
  @autoRoute
  @requiresPermissions(#[])
  op actions(...ResourceCollectionParameters<SiteEventResource>):
    | SiteEventActionsResult
    | NotFoundProblem;
}
