package broadcasts

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseBroadcastRecipientsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.BroadcastRecipient().Delete().Where(broadcastrecipient.IDIn(ids...)).Exec(ctx)
	return err
}

func EraseBroadcastsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Broadcast().Purge().Where(broadcast.IDIn(ids...)).Exec(ctx)
	return err
}
