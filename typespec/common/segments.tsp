// The Segment write bodies: /site and /api take them the same way. A Segment's Condition is in
// conditions.tsp.

namespace Sphericon;

/** Request body for creating a segment */
model CreateSegmentInput {
  /** Segment name */
  name: Name;

  /** Segment condition: the tree of tests; an empty group matches all contacts */
  condition: SegmentConditionInput;
}

/** Request body for updating a segment */
model UpdateSegmentInput {
  /** The segment's `revision` the edit was made on (ADR 0119): the one it was read with */
  revision: Revision;

  /** Segment name */
  name?: Name;

  /** Segment condition: the tree of tests; an empty group matches all contacts */
  condition?: SegmentConditionInput;
}
