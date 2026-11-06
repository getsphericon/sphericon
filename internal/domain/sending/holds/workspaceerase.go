package holds

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/hold"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseHoldsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Hold().Delete().Where(hold.IDIn(ids...)).Exec(ctx)
	return err
}
