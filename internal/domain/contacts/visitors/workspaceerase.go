package visitors

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/visitor"
)

// EraseVisitorsIn removes the scoped Workspace's Visitors with the ids, inside the transaction s is
// bound to: one batch of the erase of a deleted Workspace (internal/usecase/workspacedeletion, ADR
// 0043), which lists the ids and checks nothing else.
func EraseVisitorsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Visitor().Delete().Where(visitor.IDIn(ids...)).Exec(ctx)
	return err
}
