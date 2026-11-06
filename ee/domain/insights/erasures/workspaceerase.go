package erasures

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/erasure"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseErasuresIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Erasure().Delete().Where(erasure.IDIn(ids...)).Exec(ctx)
	return err
}
