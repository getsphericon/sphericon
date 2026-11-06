package usages

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/usage"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseUsageIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Usage().Delete().Where(usage.IDIn(ids...)).Exec(ctx)
	return err
}
