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

  /** Event action: a System event's (`SystemEventAction`) or the customer's own */
  action: string;

  /** Arbitrary event properties */
  properties?: JsonObject | null;

  /**
   * The subject the Contact received on the email the event is about, merge tags filled; null for
   * an event that is about no email, or whose email kept no subject (erased with the Contact)
   */
  emailSubject?: string | null;

  /** When the event happened: the time the source gave, or when it was published if it gave none */
  occurredAt: Timestamp;

  /** When the event was recorded */
  createdAt: Timestamp;
}

/** Distinct event actions seen in a workspace (for segment/filter pickers) */
model SiteEventActionsResult {
  actions: string[];

  /** How many days of analytical Events the Instance keeps (ADR 0019), so an event test with no
   * window, or one longer than that, counts only the Events kept; 0 means every Event is kept */
  eventsKeptDays: int32;

  /** The actions of a message's sent, delivery and engagement System events, on every Channel:
   * the Events an Event filter may test by what sent the message (`sentBy`) */
  messageActions: string[];

  /** The evidentiary System events retention never deletes by age (ADR 0019), which an event
   * test counts back to their first */
  keptActions: string[];
}

/** The property keys seen on the most recent events of an action, sorted (for the Enrollment key
 * picker): a hint, not a catalogue, since property keys have none */
model SiteEventPropertyKeysResult {
  keys: string[];
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
  ): Page<SiteEventResource> | Problem;

  /** Distinct event actions in the workspace, sorted — powers the segment builder */
  @collectionAction(SiteEventResource, "actions")
  @get
  @autoRoute
  @requiresPermissions(#[])
  op actions(...ResourceCollectionParameters<SiteEventResource>):
    | SiteEventActionsResult
    | Problem;

  /** The property keys of the most recent events of the action, distinct and sorted — powers the
   * automation Trigger's Enrollment key picker */
  @collectionAction(SiteEventResource, "property-keys")
  @get
  @autoRoute
  @requiresPermissions(#[])
  op propertyKeys(
    ...ResourceCollectionParameters<SiteEventResource>,

    /** The event action whose recent events are read */
    @query action: string,
  ): SiteEventPropertyKeysResult | Problem;
}
