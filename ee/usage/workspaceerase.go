package usage

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/usage"
)

// EraseUsageIn removes the scoped Workspace's Usage rows with the ids, inside the transaction s is
// bound to: one batch of the erase of a deleted Workspace (internal/usecase/workspacedeletion, ADR
// 0043), which lists the ids and checks nothing else.
func EraseUsageIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Usage().Delete().Where(usage.IDIn(ids...)).Exec(ctx)
	return err
}
