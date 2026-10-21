using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

namespace Sphericon;

/** How a segment group combines its rules */
enum SiteSegmentCombinator {
  and,
  or,
}

/** What a segment rule tests. A text attribute takes eq, neq, contains, beginsWith, null and
 * notNull; a custom field takes eq, neq, contains and notNull; an event takes performed and
 * notPerformed; the tag field takes has and doesNotHave. */
enum SiteSegmentOperator {
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
model SiteSegmentRule {
  kind: "rule";

  /** What it tests: a contact attribute (email, phone, subject_id, first_name, last_name,
   * time_zone), "tag", "custom:<key>" for a custom field of the workspace, or "event:<action>" */
  field: string;

  /** How the field is tested */
  operator: SiteSegmentOperator;

  /** The value to test against: a tag name, a day window for events ("" = ever), blank for
   * null and notNull */
  value?: string;
}

/** A rule or a nested group inside a segment group, discriminated by `kind`. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union SiteSegmentRuleNode {
  rule: SiteSegmentRule,
  group: SiteSegmentRuleGroup,
}

/** A group of rules combined with and/or, optionally negated. An empty group matches every
 * contact (an empty negated group matches none). */
model SiteSegmentRuleGroup {
  kind: "group";

  /** How the rules combine */
  combinator: SiteSegmentCombinator;

  /** Negate the whole group */
  not?: boolean;

  /** The group's rules and nested groups */
  rules: SiteSegmentRuleNode[];
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

  /** Segment definition: the rule group; an empty group matches all contacts */
  definition?: SiteSegmentRuleGroup;

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

  /** Segment definition: the rule group; an empty group matches all contacts */
  definition: SiteSegmentRuleGroup;
}

/** Site request body for updating a segment */
model SiteUpdateSegmentInput {
  /** Segment name */
  name?: string;

  /** Segment definition: the rule group; an empty group matches all contacts */
  definition?: SiteSegmentRuleGroup;
}

/** Request body for previewing a rule definition's audience */
model SitePreviewSegmentInput {
  /** Rule definition; absent means all contacts */
  definition?: SiteSegmentRuleGroup;
}

/** Result of a segment preview: how many deliverable contacts match */
model SitePreviewSegmentResult {
  /** Number of active contacts matching the rule */
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
