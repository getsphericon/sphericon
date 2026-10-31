using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** How a condition group combines its entries */
enum SiteConditionCombinator {
  and,
  or,
}

/** What a condition leaf tests. A text attribute takes eq, neq, contains, beginsWith, null and
 * notNull; the tag field takes has and doesNotHave; a custom field takes the operators of its type,
 * which the custom field lists as `ruleOperators` (a number custom field is also compared by a
 * number leaf, a datetime one tested by the date leaves). */
enum SiteConditionOperator {
  eq,
  neq,
  contains,
  beginsWith,
  null,
  notNull,
  has,
  doesNotHave,
}

/** One test of a condition: a field, an operator and a value */
model SiteConditionLeaf {
  kind: "rule";

  /** What it tests: a contact attribute (email, phone, subject_id, first_name, last_name,
   * time_zone), "tag", "custom:<key>" for a custom field of the workspace, or "trigger:<key>"
   * (a property of the Event that fired a rule: valid only in a rule, refused in a segment, takes the
   * text operators, and is false when the Event lacks the key or holds a value that is not text; a
   * number property is compared by a number leaf) */
  field: string;

  /** How the field is tested */
  operator: SiteConditionOperator;

  /** The value to test against: a tag name, blank for null and notNull */
  value?: string;
}

/** How an event leaf bounds the count: at least `times`, or at most `times` */
enum SiteConditionEventOperator {
  atLeast,
  atMost,
}

/** A test of how many times a contact performed an Event action, over all time or within the
 * last `withinDays` days. Performed is atLeast 1, not performed is atMost 0; "exactly" is a group of
 * both. Engagement Events made by software (machine opens and clicks) are not counted. A window
 * longer than Event retention is capped by it: expired Events are no longer counted. */
model SiteConditionEventLeaf {
  kind: "event";

  /** The Event action counted */
  action: string;

  /** How the count is bounded */
  operator: SiteConditionEventOperator;

  /** The bound: atLeast takes 1 or more, atMost 0 or more */
  @minValue(0)
  times: int32;

  /** Count only the Events of the last this many days; absent counts all time */
  @minValue(1)
  withinDays?: int32;
}

/** How a number leaf compares the field with `value`: greater than, greater than or equal,
 * less than, or less than or equal */
enum SiteConditionNumberOperator {
  gt,
  gte,
  lt,
  lte,
}

/** A comparison of a number: a custom field of the workspace whose type is number
 * ("custom:<key>"), or a property of the Event that fired a rule ("trigger:<key>", valid only in a
 * rule). A missing key, or a value that is not a number (text, a boolean, null, a list), makes the
 * leaf false. */
model SiteConditionNumberLeaf {
  kind: "number";

  /** What it compares: "custom:<key>" for a number custom field, or "trigger:<key>" */
  field: string;

  /** How the field compares with `value` */
  operator: SiteConditionNumberOperator;

  /** The number the field is compared with */
  value: float64;
}

/** Which side of `at` a date leaf takes: before is strictly earlier, after is at or later, so the
 * two split time at `at` */
enum SiteConditionDateOperator {
  before,
  after,
}

/** A test of a custom field of the workspace whose type is datetime ("custom:<key>"): its moment
 * is before or after `at`. Only a date-time with a UTC offset is a moment: a missing key, a date
 * without a time, a date-time without an offset or a value that is not a date-time makes the leaf
 * false. */
model SiteConditionDateLeaf {
  kind: "date";

  /** What it tests: "custom:<key>" for a datetime custom field */
  field: string;

  /** Which side of `at` the moment is on */
  operator: SiteConditionDateOperator;

  /** The moment the field's is compared with */
  at: Timestamp;
}

/** A test of a custom field of the workspace whose type is datetime ("custom:<key>"): its moment
 * is within the last `withinDays` days up to now, so a moment still to come is not within. Only a
 * date-time with a UTC offset is a moment, as for the date leaf. */
model SiteConditionDateWithinLeaf {
  kind: "dateWithin";

  /** What it tests: "custom:<key>" for a datetime custom field */
  field: string;

  /** The window, in days back from now */
  @minValue(1)
  withinDays: int32;
}

/** A leaf (a test of a field, an event count, a comparison of a number or of a date) or a nested
 * group inside a condition group, discriminated by `kind`. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union SiteConditionNode {
  rule: SiteConditionLeaf,
  event: SiteConditionEventLeaf,
  number: SiteConditionNumberLeaf,
  date: SiteConditionDateLeaf,
  dateWithin: SiteConditionDateWithinLeaf,
  group: SiteConditionGroup,
}

/** A group of tests combined with and/or, optionally negated. An empty group matches every
 * contact (an empty negated group matches none). */
model SiteConditionGroup {
  kind: "group";

  /** How the entries combine */
  combinator: SiteConditionCombinator;

  /** Negate the whole group */
  not?: boolean;

  /** The group's leaves and nested groups */
  rules: SiteConditionNode[];
}

/** Segment resource used by the site UI */
@resource("segments")
@segment("segments")
@parentResource(SiteWorkspaceResource)
model SiteSegmentResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** Segment name */
  name: string;

  /** Segment condition: the tree of tests; an empty group matches all contacts */
  condition?: SiteConditionGroup;

  /** When the segment was archived; null while it is kept */
  archivedAt?: Timestamp | null;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Site request body for creating a segment */
model SiteCreateSegmentInput {
  /** Segment name */
  name: string;

  /** Segment condition: the tree of tests; an empty group matches all contacts */
  condition: SiteConditionGroup;
}

/** Site request body for updating a segment */
model SiteUpdateSegmentInput {
  /** Segment name */
  name?: string;

  /** Segment condition: the tree of tests; an empty group matches all contacts */
  condition?: SiteConditionGroup;
}

/** Request body for previewing a condition's segment */
model SitePreviewSegmentInput {
  /** Condition; absent means all contacts */
  condition?: SiteConditionGroup;
}

/** Result of a segment preview: how many deliverable contacts match */
model SitePreviewSegmentResult {
  /** Number of active contacts matching the condition */
  count: int32;
}

@tag("Site Segments")
interface SiteSegments
  extends WorkspaceCrudArchivable<
      SiteSegmentResource,
      SiteCreateSegmentInput,
      SiteUpdateSegmentInput
    > {
  /** Preview how many active contacts match a rule definition */
  @collectionAction(SiteSegmentResource, "preview")
  @autoRoute
  @requiresPermissions(#[])
  preview(
    ...ResourceCollectionParameters<SiteSegmentResource>,
    @body body: SitePreviewSegmentInput,
  ):
    | SitePreviewSegmentResult
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;
}

@route("/workspaces/{workspaceId}/segments/{segmentId}/contacts")
@tag("Site Segments")
namespace SiteSegmentContacts {
  /**
   * List the contacts a segment matches right now, evaluated live, ascending by id. A stored
   * condition that no longer fits the contact schema is a 422.
   */
  @get
  @requiresPermissions(#[])
  op list(
    @path workspaceId: WorkspaceId,
    @path segmentId: EntityId,
    ...PageQuery,
  ):
    | Page<SiteContactResource>
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;
}

@@requiresPermissions(SiteSegments.archive, #[]);
@@requiresPermissions(SiteSegments.create, #[]);
@@requiresPermissions(SiteSegments.get, #[]);
@@requiresPermissions(SiteSegments.list, #[]);
@@requiresPermissions(SiteSegments.unarchive, #[]);
@@requiresPermissions(SiteSegments.update, #[]);
