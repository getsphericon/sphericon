package broadcasts

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/condition"
)

// UsingTag returns the armed Broadcasts (scheduled, or sending and so still to be planned) whose
// copied Condition names the Tag by its name, in id order (ADR 0065). The copy keeps the name, not
// an id, so deleting the Tag would change who they reach. A draft follows its Segment, which
// answers for itself; a finished Broadcast resolves nothing any more.
func UsingTag(ctx context.Context, s *ent.Scoped, name string) ([]*ent.Broadcast, error) {
	armed, err := s.Broadcast().Query().
		Where(broadcast.StatusIn(broadcast.StatusScheduled, broadcast.StatusSending), broadcast.ConditionNotNil()).
		Order(ent.Asc(broadcast.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return lo.Filter(armed, func(b *ent.Broadcast, _ int) bool {
		g, err := condition.Parse(*b.Condition)
		return err == nil && g.NamesTag(name)
	}), nil
}
