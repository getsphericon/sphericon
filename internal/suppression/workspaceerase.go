package suppression

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/suppression"
)

// EraseSuppressionsIn removes the scoped Workspace's Suppressions with the ids, inside the
// transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseSuppressionsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Suppression().Delete().Where(suppression.IDIn(ids...)).Exec(ctx)
	return err
}
