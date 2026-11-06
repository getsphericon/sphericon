package unsubscribes

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseUnsubscribesIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Unsubscribe().Delete().Where(unsubscribe.IDIn(ids...)).Exec(ctx)
	return err
}
