package segments

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/segment"
	"github.com/getsphericon/sphericon/internal/condition"
)

// UsingTag returns the Workspace's Segments whose Condition names the Tag by its name, in id order.
// Archived ones count: restoring one must not leave it naming a missing Tag. A Condition keeps the
// name, not an id, so deleting the Tag would leave it matching nothing.
func UsingTag(ctx context.Context, s *ent.Scoped, name string) ([]*ent.Segment, error) {
	all, err := s.Segment().QueryWithDiscarded().Order(ent.Asc(segment.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	var using []*ent.Segment
	for _, seg := range all {
		g, err := condition.Parse(lo.FromPtr(seg.Condition))
		if err == nil && g.NamesTag(name) {
			using = append(using, seg)
		}
	}
	return using, nil
}
