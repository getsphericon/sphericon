package providerlimit

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/providerlimiter"
	"github.com/getsphericon/sphericon/ent/sendcaplimiter"
)

// EraseProviderLimitersIn removes the scoped Workspace's provider limiter buckets with the ids,
// inside the transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseProviderLimitersIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.ProviderLimiter().Delete().Where(providerlimiter.IDIn(ids...)).Exec(ctx)
	return err
}

// EraseSendCapLimitersIn removes the scoped Workspace's Send cap buckets with the ids, inside the
// transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseSendCapLimitersIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.SendCapLimiter().Delete().Where(sendcaplimiter.IDIn(ids...)).Exec(ctx)
	return err
}
