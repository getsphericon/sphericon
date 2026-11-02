package broadcasts

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/condition"
)

// UnfitForCustomFieldType returns the armed Broadcasts (scheduled, or sending and so still to be
// planned), in id order, whose copied Condition tests the Custom field key with a leaf a field of
// type t does not take (ADR 0065): retyping the field would fail their planning. A draft follows
// its Segment, which answers for itself.
func UnfitForCustomFieldType(ctx context.Context, s *ent.Scoped, key string, t customfield.Type) ([]*ent.Broadcast, error) {
	armed, err := s.Broadcast().Query().
		Where(broadcast.StatusIn(broadcast.StatusScheduled, broadcast.StatusSending), broadcast.ConditionNotNil()).
		Order(ent.Asc(broadcast.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return lo.Filter(armed, func(b *ent.Broadcast, _ int) bool {
		g, err := condition.Parse(*b.Condition)
		return err == nil && !g.FitsCustomFieldType(key, t)
	}), nil
}
