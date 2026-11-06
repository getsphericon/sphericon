package suppressions

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/suppression"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseSuppressionsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Suppression().Delete().Where(suppression.IDIn(ids...)).Exec(ctx)
	return err
}
