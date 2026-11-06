package confirmations

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseConfirmationsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Confirmation().Delete().Where(confirmation.IDIn(ids...)).Exec(ctx)
	return err
}
