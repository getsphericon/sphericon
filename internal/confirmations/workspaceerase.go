package confirmations

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
)

// EraseConfirmationsIn removes the scoped Workspace's Confirmations with the ids, inside the
// transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseConfirmationsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Confirmation().Delete().Where(confirmation.IDIn(ids...)).Exec(ctx)
	return err
}
