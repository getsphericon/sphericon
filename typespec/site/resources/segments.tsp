using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

namespace Sphericon;

/** How a condition group combines its entries */
enum SiteConditionCombinator {
  and,
  or,
}

/** What a condition leaf tests. A text attribute takes eq, neq, contains, beginsWith, null and
 * notNull; a custom field takes eq, neq, contains and notNull; an event takes performed and
 * notPerformed; the tag field takes has and doesNotHave. */
enum SiteConditionOperator {
  eq,
  neq,
  contains,
  beginsWith,
  null,
  notNull,
  performed,
  notPerformed,
  has,
  doesNotHave,
}

/** One test of a condition: a field, an operator and a value */
model SiteConditionLeaf {
  kind: "rule";

  /** What it tests: a contact attribute (email, phone, subject_id, first_name, last_name,
   * time_zone), "tag", "custom:<key>" for a custom field of the workspace, "event:<action>", or "trigger:<key>"
   * (a property of the Event that fired a rule: valid only in a rule, refused in a segment, takes the
   * text operators, and is false when the Event lacks the key or holds a value that is not text) */
  field: string;

  /** How the field is tested */
  operator: SiteConditionOperator;

  /** The value to test against: a tag name, a day window for events ("" = ever), blank for
   * null and notNull */
  value?: string;
}

/** A leaf or a nested group inside a condition group, discriminated by `kind`. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union SiteConditionNode {
  rule: SiteConditionLeaf,
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

/** Request body for previewing a condition's audience */
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
  /** List the contacts a segment matches right now, evaluated live, ascending by id */
  @get
  op list(
    @path workspaceId: WorkspaceId,
    @path segmentId: EntityId,
    ...PageQuery,
  ): Page<SiteContactResource> | BadRequestProblem | NotFoundProblem;
}
