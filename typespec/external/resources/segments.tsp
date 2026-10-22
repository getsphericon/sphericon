using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

namespace Sphericon;

/** How a condition group combines its entries */
enum ConditionCombinator {
  and,
  or,
}

/** What a condition leaf tests. A text attribute takes eq, neq, contains, beginsWith, null and
 * notNull; a custom field takes eq, neq, contains and notNull; an event takes performed and
 * notPerformed; the tag field takes has and doesNotHave. */
enum ConditionOperator {
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
model ConditionLeaf {
  kind: "rule";

  /** What it tests: a contact attribute (email, phone, subject_id, first_name, last_name,
   * time_zone), "tag", "custom:<key>" for a custom field of the workspace, or "event:<action>" */
  field: string;

  /** How the field is tested */
  operator: ConditionOperator;

  /** The value to test against: a tag name, a day window for events ("" = ever), blank for
   * null and notNull */
  value?: string;
}

/** A leaf or a nested group inside a condition group, discriminated by `kind`. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union ConditionNode {
  rule: ConditionLeaf,
  group: ConditionGroup,
}

/** A group of tests combined with and/or, optionally negated. An empty group matches every
 * contact (an empty negated group matches none). */
model ConditionGroup {
  kind: "group";

  /** How the entries combine */
  combinator: ConditionCombinator;

  /** Negate the whole group */
  not?: boolean;

  /** The group's leaves and nested groups */
  rules: ConditionNode[];
}

/** Segment resource */
@resource("segments")
@segment("segments")
model SegmentResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** Segment name */
  name: string;

  /** Segment condition: the tree of tests; an empty group matches all contacts */
  condition?: ConditionGroup;

  /** When the segment was archived; null while it is kept */
  archivedAt?: Timestamp | null;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Request body for creating a segment */
model CreateSegmentInput {
  /** Segment name */
  name: string;

  /** Segment condition: the tree of tests; an empty group matches all contacts */
  condition: ConditionGroup;
}

/** Request body for updating a segment */
model UpdateSegmentInput {
  /** Segment name */
  name?: string;

  /** Segment condition: the tree of tests; an empty group matches all contacts */
  condition?: ConditionGroup;
}

/** Request body for previewing a condition's audience */
model PreviewSegmentInput {
  /** Condition; absent means all contacts */
  condition?: ConditionGroup;
}

/** Result of a segment preview */
model PreviewSegmentResult {
  /** Number of contacts matching the condition */
  count: int32;
}

@tag("Segments")
interface Segments
  extends ExternalCrudArchivable<
      SegmentResource,
      CreateSegmentInput,
      UpdateSegmentInput
    > {
  /** Count the contacts matching an unsaved rule definition */
  @collectionAction(SegmentResource, "preview")
  @autoRoute
  preview(
    ...ResourceCollectionParameters<SegmentResource>,
    @body body: PreviewSegmentInput,
  ):
    | PreviewSegmentResult
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | UnprocessableEntityProblem;
}
