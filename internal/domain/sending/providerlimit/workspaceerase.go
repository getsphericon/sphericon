package providerlimit

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/providerlimiter"
	"github.com/getsphericon/sphericon/ent/sendcaplimiter"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseProviderLimitersIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.ProviderLimiter().Delete().Where(providerlimiter.IDIn(ids...)).Exec(ctx)
	return err
}

func EraseSendCapLimitersIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.SendCapLimiter().Delete().Where(sendcaplimiter.IDIn(ids...)).Exec(ctx)
	return err
}
