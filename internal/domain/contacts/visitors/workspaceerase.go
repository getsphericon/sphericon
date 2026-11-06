package visitors

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/visitor"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseVisitorsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Visitor().Delete().Where(visitor.IDIn(ids...)).Exec(ctx)
	return err
}
