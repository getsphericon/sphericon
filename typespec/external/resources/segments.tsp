using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

namespace Sphericon;

/** How a segment group combines its rules */
enum SegmentCombinator {
  and,
  or,
}

/** What a segment rule tests. A text attribute takes eq, neq, contains, beginsWith, null and
 * notNull; a custom field takes eq, neq, contains and notNull; an event takes performed and
 * notPerformed; the tag field takes has and doesNotHave. */
enum SegmentOperator {
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

/** One condition of a segment: a field, an operator and a value */
model SegmentRule {
  kind: "rule";

  /** What it tests: a contact attribute (email, phone, subject_id, first_name, last_name,
   * time_zone), "tag", "custom:<key>" for a custom field of the workspace, or "event:<action>" */
  field: string;

  /** How the field is tested */
  operator: SegmentOperator;

  /** The value to test against: a tag name, a day window for events ("" = ever), blank for
   * null and notNull */
  value?: string;
}

/** A rule or a nested group inside a segment group, discriminated by `kind`. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union SegmentRuleNode {
  rule: SegmentRule,
  group: SegmentRuleGroup,
}

/** A group of rules combined with and/or, optionally negated. An empty group matches every
 * contact (an empty negated group matches none). */
model SegmentRuleGroup {
  kind: "group";

  /** How the rules combine */
  combinator: SegmentCombinator;

  /** Negate the whole group */
  not?: boolean;

  /** The group's rules and nested groups */
  rules: SegmentRuleNode[];
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

  /** Segment definition: the rule group; an empty group matches all contacts */
  definition?: SegmentRuleGroup;

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

  /** Segment definition: the rule group; an empty group matches all contacts */
  definition: SegmentRuleGroup;
}

/** Request body for updating a segment */
model UpdateSegmentInput {
  /** Segment name */
  name?: string;

  /** Segment definition: the rule group; an empty group matches all contacts */
  definition?: SegmentRuleGroup;
}

/** Request body for previewing a rule definition's audience */
model PreviewSegmentInput {
  /** Rule definition; absent means all contacts */
  definition?: SegmentRuleGroup;
}

/** Result of a segment preview */
model PreviewSegmentResult {
  /** Number of contacts matching the rule */
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
