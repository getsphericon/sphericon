package segments

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/segment"
	"github.com/getsphericon/sphericon/internal/condition"
)

// UnfitForCustomFieldType returns the Workspace's Segments, in id order, whose Condition tests the
// Custom field key with a leaf a field of type t does not take, so they would stop compiling were
// the field retyped. Archived ones count, as for a Tag: restoring one must not bring back a
// Condition that no longer compiles.
func UnfitForCustomFieldType(ctx context.Context, s *ent.Scoped, key string, t customfield.Type) ([]*ent.Segment, error) {
	all, err := s.Segment().QueryWithDiscarded().Order(ent.Asc(segment.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	return lo.Filter(all, func(seg *ent.Segment, _ int) bool {
		g, err := condition.Parse(lo.FromPtr(seg.Condition))
		return err == nil && !g.FitsCustomFieldType(key, t)
	}), nil
}
