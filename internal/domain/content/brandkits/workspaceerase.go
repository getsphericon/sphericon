package brandkits

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/brandkit"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseBrandKitsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.BrandKit().Delete().Where(brandkit.IDIn(ids...)).Exec(ctx)
	return err
}
