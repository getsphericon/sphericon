package brandkits

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/brandkit"
)

// EraseBrandKitsIn removes the scoped Workspace's Brand kit with the ids, inside the transaction s
// is bound to: one batch of the erase of a deleted Workspace (internal/usecase/workspacedeletion,
// ADR 0043), which lists the ids and checks nothing else.
func EraseBrandKitsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.BrandKit().Delete().Where(brandkit.IDIn(ids...)).Exec(ctx)
	return err
}
