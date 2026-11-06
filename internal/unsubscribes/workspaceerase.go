package unsubscribes

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
)

// EraseUnsubscribesIn removes the scoped Workspace's Unsubscribes with the ids, inside the
// transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseUnsubscribesIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Unsubscribe().Delete().Where(unsubscribe.IDIn(ids...)).Exec(ctx)
	return err
}
